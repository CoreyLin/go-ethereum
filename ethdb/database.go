// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// +build !js

package ethdb

// 引入了github.com/syndtr/goleveldb/leveldb package，go封装过的leveldb，go语言版本的访问leveldb的package，源码在github上，是一个通用，
// 基础的包，用于对leveldb进行增删改查等基础操作。ethdb package在leveldb的基础上又进行了一次封装，但封装的功能还是很通用和基础的功能，
// 并没有直接和以太坊业务相关。
import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	writePauseWarningThrottler = 1 * time.Minute
)

// 字面意思是限制能够打开的数据库的数量，由于leveldb是把数据存在硬盘的文件上的，所以这里就是能够打开的文件的数量限制，准确的说，是文件夹，一个文件夹对应一个数据库。
// 但这个变量没有被其他地方用到，不知为何。go语言里面，是不允许没有被使用的本地变量存在的，但是OpenFileLimit变量是一个global变量，虽然没有被使用，还是可以放在这里，
// 编译不会报错。
var OpenFileLimit = 64

// LDBDatabase在第三方leveldb.DB的基础上又进行了封装
type LDBDatabase struct {
	fn string      // filename for reporting 实际上就是数据库的名字，也就是数据库对应的文件夹的名字
	db *leveldb.DB // LevelDB instance leveldb的实例，这个leveldb更加底层和通用

	// 一堆meter，是以太坊用于统计数据，评估性能的Meter，meter中文是仪表，了解一下就行
	compTimeMeter    metrics.Meter // Meter for measuring the total time spent in database compaction 测量在leveldb里压缩数据需要的时间
	compReadMeter    metrics.Meter // Meter for measuring the data read during compaction 测量在压缩期间读入的数据
	compWriteMeter   metrics.Meter // Meter for measuring the data written during compaction 测量在压缩期间写入的数据
	writeDelayNMeter metrics.Meter // Meter for measuring the write delay number due to database compaction 测量由于数据压缩引起的写数据延迟条数
	writeDelayMeter  metrics.Meter // Meter for measuring the write delay duration due to database compaction 测量由于数据压缩引起的写数据延迟时间
	diskReadMeter    metrics.Meter // Meter for measuring the effective amount of data read 测量有效的数据读入量
	diskWriteMeter   metrics.Meter // Meter for measuring the effective amount of data written 测量有效的数据写入量

	quitLock sync.Mutex      // Mutex protecting the quit channel access 用于关闭数据库时的互斥保护，同一时刻只能有一个线程执行Close函数
	quitChan chan chan error // Quit channel to stop the metrics collection before closing the database
							// 调用Close函数的时候quitChan就会接收chan error，然后发送给meter里面的errc

	log log.Logger // Contextual logger tracking the database path 带上下文的logger
}

// NewLDBDatabase returns a LevelDB wrapped object.
// NewLDBDatabase返回LevelDB包装对象。
func NewLDBDatabase(file string, cache int, handles int) (*LDBDatabase, error) {
	logger := log.New("database", file) // New一个带上下文的logger，每次打印的时候都会把database的名字打印出来

	// Ensure we have some minimal caching and file guarantees
	if cache < 16 {
		cache = 16 // 确保最小的cache是16，如果不足16，设置为16
	}
	if handles < 16 {
		handles = 16 // 确保最小的handles是16，如果不足16，设置为16
	}
	logger.Info("Allocated cache and file handles", "cache", cache, "handles", handles)

	// Open the db and recover any potential corruptions
	// https://godoc.org/github.com/syndtr/goleveldb/leveldb#OpenFile 如果发现DB里面有数据损坏就返回结构体ErrCorrupted的一个实例，
	// 此结构体实现了error接口，如果返回了ErrCorrupted，可以用Recover函数恢复
	db, err := leveldb.OpenFile(file, &opt.Options{
		// 参考https://godoc.org/github.com/syndtr/goleveldb/leveldb/opt#Options
		OpenFilesCacheCapacity: handles, // TODO 疑问，究竟是用来干啥的？
		BlockCacheCapacity:     cache / 2 * opt.MiB, // TODO 疑问，究竟是用来干啥的？
		// WriteBuffer defines maximum size of a 'memdb' before flushed to 'sorted table'. 'memdb' is an in-memory DB backed by an on-disk
		// unsorted journal.LevelDB may held up to two 'memdb' at the same time.The default value is 4MiB. TODO 还需要弄得更清楚
		WriteBuffer:            cache / 4 * opt.MiB, // Two of these are used internally
		// Filter defines an 'effective filter' to use. An 'effective filter' if defined will be used to generate per-table filter block.
		// The filter name will be stored on disk.During reads LevelDB will try to find matching filter from 'effective filter' and
		// 'alternative filters'.Filter can be changed after a DB has been created. It is recommended to put old filter to the
		// 'alternative filters' to mitigate lack of filter during transition period.A filter is used to reduce disk reads
		// when looking for a specific key.The default value is nil.
		Filter:                 filter.NewBloomFilter(10),
	})
	// https://golang.org/ref/spec#Type_assertions
	// 如果err的确是ErrCorrupted类型的，那么corrupted的值就是true，否则就是false
	if _, corrupted := err.(*errors.ErrCorrupted); corrupted {
		// https://godoc.org/github.com/syndtr/goleveldb/leveldb#RecoverFile 恢复损坏的数据库
		db, err = leveldb.RecoverFile(file, nil)
	}
	// (Re)check for errors and abort if opening of the db failed 再检查一次err的值，如果不等于nil，说明打开数据库失败
	if err != nil {
		return nil, err
	}
	return &LDBDatabase{
		fn:  file, // 数据库名称
		db:  db, // 内部的db实例
		log: logger, // 带context的log
	}, nil // error是nil，说明没有错误
	// LDBDatabase还有很多其他属性，比如Meter，Mutex,chan等等，但是这里没有指定
}

// Path returns the path to the database directory. 返回数据库对应的文件夹的路径
func (db *LDBDatabase) Path() string {
	return db.fn
}

// Put puts the given key / value to the queue 向数据库中增加或者修改键值对
func (db *LDBDatabase) Put(key []byte, value []byte) error {
	return db.db.Put(key, value, nil)
}

// 判断数据库中是否有参数指定的键
func (db *LDBDatabase) Has(key []byte) (bool, error) {
	return db.db.Has(key, nil)
}

// Get returns the given key if it's present. 如果键存在，返回键对应的值
func (db *LDBDatabase) Get(key []byte) ([]byte, error) {
	dat, err := db.db.Get(key, nil)
	if err != nil {
		return nil, err
	}
	return dat, nil
}

// Delete deletes the key from the queue and database 从数据库中删除指定的键值对
func (db *LDBDatabase) Delete(key []byte) error {
	return db.db.Delete(key, nil)
}

// 返回数据库最新snapshot（快照）的一个迭代器
func (db *LDBDatabase) NewIterator() iterator.Iterator {
	return db.db.NewIterator(nil, nil)
}

// NewIteratorWithPrefix returns a iterator to iterate over subset of database content with a particular prefix.
// 同样返回数据库最新快照的一个迭代器，但是只迭代指定前缀的键
func (db *LDBDatabase) NewIteratorWithPrefix(prefix []byte) iterator.Iterator {
	return db.db.NewIterator(util.BytesPrefix(prefix), nil)
}

func (db *LDBDatabase) Close() {
	// Stop the metrics collection to avoid internal database races 这个函数不能并行执行，相当于加锁保证同步
	db.quitLock.Lock()
	defer db.quitLock.Unlock()

	if db.quitChan != nil {
		errc := make(chan error)
		db.quitChan <- errc
		if err := <-errc; err != nil {
			db.log.Error("Metrics collection failed", "err", err)
		}
		db.quitChan = nil
	}
	err := db.db.Close()
	if err == nil {
		db.log.Info("Database closed")
	} else {
		db.log.Error("Failed to close database", "err", err)
	}
}

// 返回LDBDatabase的db属性，真正用于和leveldb通信的实例
func (db *LDBDatabase) LDB() *leveldb.DB {
	return db.db
}

// Meter configures the database metrics collectors and 配置数据库度量/指标收集器
func (db *LDBDatabase) Meter(prefix string) {
	// Initialize all the metrics collector at the requested prefix
	db.compTimeMeter = metrics.NewRegisteredMeter(prefix+"compact/time", nil)
	db.compReadMeter = metrics.NewRegisteredMeter(prefix+"compact/input", nil)
	db.compWriteMeter = metrics.NewRegisteredMeter(prefix+"compact/output", nil)
	db.diskReadMeter = metrics.NewRegisteredMeter(prefix+"disk/read", nil)
	db.diskWriteMeter = metrics.NewRegisteredMeter(prefix+"disk/write", nil)
	db.writeDelayMeter = metrics.NewRegisteredMeter(prefix+"compact/writedelay/duration", nil)
	db.writeDelayNMeter = metrics.NewRegisteredMeter(prefix+"compact/writedelay/counter", nil)

	// Create a quit channel for the periodic collector and run it 为周期性的度量收集器创建一个退出通道
	db.quitLock.Lock()
	db.quitChan = make(chan chan error)
	db.quitLock.Unlock()

	go db.meter(3 * time.Second) // 新开一个goroutine来启动database的度量收集，每3秒钟收集一次
}

// meter periodically retrieves internal leveldb counters and reports them to
// the metrics subsystem.
// 周期性地收集leveldb内部的各种计数器的值，并且上报给度量子系统
//
// This is how a stats table look like (currently):
// 统计表格就像下面这样子（db.db.GetProperty("leveldb.stats")）:
//   Compactions
//    Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
//   -------+------------+---------------+---------------+---------------+---------------
//      0   |          0 |       0.00000 |       1.27969 |       0.00000 |      12.31098
//      1   |         85 |     109.27913 |      28.09293 |     213.92493 |     214.26294
//      2   |        523 |    1000.37159 |       7.26059 |      66.86342 |      66.77884
//      3   |        570 |    1113.18458 |       0.00000 |       0.00000 |       0.00000
//
// This is how the write delay look like (currently):
// 写入延迟是这样呈现的：
// DelayN:5 Delay:406.604657ms Paused: false
//
// This is how the iostats look like (currently):
// 输入输出统计是这样呈现的：
// Read(MB):3895.04860 Write(MB):3654.64712
// 读了多少M的数据，写入了多少M的数据
func (db *LDBDatabase) meter(refresh time.Duration) {
	// Create the counters to store current and previous compaction values
	// 创建一个二维切片来存储现在和以前压缩的信息，长度和容量都为2，每个元素又是一个float64的切片
	compactions := make([][]float64, 2)
	for i := 0; i < 2; i++ {
		// 把二维切片compactions里面的每个元素设置为一个长度和容量都为3的float64切片
		compactions[i] = make([]float64, 3)
	}
	// Create storage for iostats.
	// 创建一个长度为2的float64数组来存储iostats信息
	var iostats [2]float64

	// Create storage and warning log tracer for write delay.
	var (
		delaystats      [2]int64 // 声明一个长度为2的int64数组来存储写入延迟的统计数据，第一个元素存储延迟的累计次数，比如DelayN:5
								// 第二个元素存储延迟的累计时间Delay:406.604657ms
		lastWritePaused time.Time // 上次写入停止的时间点，可能会被打印到日志里面
	)

	var (
		errc chan error // errc是一个通道，通道里面可以装error类型的数据
		merr error // // 表示度量收集过程中遇到的error
	)

	// Iterate ad infinitum and collect the stats
	// 以无限循环的方式周期性地收集数据库的统计信息
	for i := 1; errc == nil && merr == nil; i++ {
		// Retrieve the database stats 返回底层数据库的统计信息
		stats, err := db.db.GetProperty("leveldb.stats")
		if err != nil {
			db.log.Error("Failed to read database stats", "err", err)
			merr = err
			continue
		}
		// Find the compaction table, skip the header 找到压缩表，跳过header（头部）
		lines := strings.Split(stats, "\n") // 返回的是字符串，用换行符分隔为字符串切片
		for len(lines) > 0 && strings.TrimSpace(lines[0]) != "Compactions" {
			// 如果字符串切片的第一个元素不等于Compactions，那么就把第一个元素剔除
			lines = lines[1:]
		}
		if len(lines) <= 3 { // 如果字符串切片长度小于等于3，说明没有压缩表
			db.log.Error("Compaction table not found")
			merr = errors.New("compaction table not found")
			continue
		}
		lines = lines[3:] // 把字符串切片的前三个元素剔除，只留第四个元素开始的元素

		// Iterate over all the table rows, and accumulate the entries
		// i为奇数时，把compaction[1][0],compaction[1][1],compaction[1][2]的值都设置为0，即清零，重新读取统计信息
		// i为偶数时，把compaction[0][0],compaction[0][1],compaction[0][2]的值都设置为0，即清零，重新读取统计信息
		for j := 0; j < len(compactions[i%2]); j++ {
			compactions[i%2][j] = 0
		}
		for _, line := range lines {
			// 对lines里面的每一个string元素用竖线分割为一个切片
			parts := strings.Split(line, "|")
			// 如果用竖线分隔之后的切片的长度不是6，就跳出循环
			if len(parts) != 6 {
				break
			}
			// 只取切片的第四个元素开始的元素，分别代表time，read，write
			for idx, counter := range parts[3:] {
				value, err := strconv.ParseFloat(strings.TrimSpace(counter), 64)
				if err != nil {
					db.log.Error("Compaction entry parsing failed", "err", err)
					merr = err
					continue
				}
				// i为奇数的话，就设置compactions[1]的值；i为偶数的话，就设置compactions[0]的值
				// idx为0，代表time；idx为1，代表read；idx为2，代表write
				// 注意：这里是+=，即累加，对每一个level的time，read，write分别累加
				compactions[i%2][idx] += value
			}
		}
		// Update all the requested meters 更新和压缩表相关的度量值
		if db.compTimeMeter != nil { // 已经在Meter函数里面初始化过了，所以不会是nil
			// Mark操作是累加操作
			// compactions[i%2][0]代表这一次的值，compactions[(i-1)%2][0]代表上一次的值，相减就是这一次和上一次的差值，由于压缩表里的时间
			// 单位是秒，这里转换成了纳秒
			db.compTimeMeter.Mark(int64((compactions[i%2][0] - compactions[(i-1)%2][0]) * 1000 * 1000 * 1000))
		}
		if db.compReadMeter != nil {
			// 把read的值MB转换成了字节，byte
			db.compReadMeter.Mark(int64((compactions[i%2][1] - compactions[(i-1)%2][1]) * 1024 * 1024))
		}
		if db.compWriteMeter != nil {
			// 把write的值MB转换成了字节，byte
			db.compWriteMeter.Mark(int64((compactions[i%2][2] - compactions[(i-1)%2][2]) * 1024 * 1024))
		}

		// Retrieve the write delay statistic 获取由于压缩造成的累计写入延迟
		writedelay, err := db.db.GetProperty("leveldb.writedelay")
		if err != nil {
			db.log.Error("Failed to read database write delay statistic", "err", err)
			merr = err
			continue
		}
		var (
			delayN        int64
			delayDuration string
			duration      time.Duration
			paused        bool
		)
		// 从writedelay字符串里面解析出&delayN, &delayDuration, &paused，比如DelayN:5 Delay:406.604657ms Paused: false
		// 至于解析出来三个变量的格式，参见https://golang.org/pkg/fmt/
		if n, err := fmt.Sscanf(writedelay, "DelayN:%d Delay:%s Paused:%t", &delayN, &delayDuration, &paused); n != 3 || err != nil {
			db.log.Error("Write delay statistic not found")
			merr = err
			continue
		}
		duration, err = time.ParseDuration(delayDuration)
		if err != nil {
			db.log.Error("Failed to parse delay duration", "err", err)
			merr = err
			continue
		}
		if db.writeDelayNMeter != nil {
			// delaystats[0]就是专门用来存储累计延迟次数的，这里做了累加操作
			db.writeDelayNMeter.Mark(delayN - delaystats[0])
		}
		if db.writeDelayMeter != nil {
			// delaystats[1]就是专门用来存储累计延迟时间纳秒数的，这里做了累加操作
			db.writeDelayMeter.Mark(duration.Nanoseconds() - delaystats[1])
		}
		// If a warning that db is performing compaction has been displayed, any subsequent
		// warnings will be withheld for one minute not to overwhelm the user.
		// 如果数据库整个写入操作停止了，并且这一次的累计延迟次数和上一次的累计延迟次数没有增加，这一次的累计延迟时间和上一次的累计延迟时间也没有增加，
		// 并且当前时间已经比上次写入停止时间过去了一分钟，那么就说明数据库还在做压缩，并且导致写入一直停止，性能下降
		if paused && delayN-delaystats[0] == 0 && duration.Nanoseconds()-delaystats[1] == 0 &&
			time.Now().After(lastWritePaused.Add(writePauseWarningThrottler)) {
			db.log.Warn("Database compacting, degraded performance")
			lastWritePaused = time.Now()
		}
		delaystats[0], delaystats[1] = delayN, duration.Nanoseconds()

		// Retrieve the database iostats.
		ioStats, err := db.db.GetProperty("leveldb.iostats")
		if err != nil {
			db.log.Error("Failed to read database iostats", "err", err)
			merr = err
			continue
		}
		var nRead, nWrite float64
		parts := strings.Split(ioStats, " ") // ioStats格式：Read(MB):3895.04860 Write(MB):3654.64712
		if len(parts) < 2 {
			db.log.Error("Bad syntax of ioStats", "ioStats", ioStats)
			merr = fmt.Errorf("bad syntax of ioStats %s", ioStats)
			continue
		}
		if n, err := fmt.Sscanf(parts[0], "Read(MB):%f", &nRead); n != 1 || err != nil {
			db.log.Error("Bad syntax of read entry", "entry", parts[0])
			merr = err
			continue
		}
		if n, err := fmt.Sscanf(parts[1], "Write(MB):%f", &nWrite); n != 1 || err != nil {
			db.log.Error("Bad syntax of write entry", "entry", parts[1])
			merr = err
			continue
		}
		if db.diskReadMeter != nil {
			db.diskReadMeter.Mark(int64((nRead - iostats[0]) * 1024 * 1024))
		}
		if db.diskWriteMeter != nil {
			db.diskWriteMeter.Mark(int64((nWrite - iostats[1]) * 1024 * 1024))
		}
		iostats[0], iostats[1] = nRead, nWrite

		// Sleep a bit, then repeat the stats collection
		select {
		case errc = <-db.quitChan:
			// Quit requesting, stop hammering the database
			// 当db.quitChan里面没有装载chan error数据的时候，这个case被阻塞，如果下一条case <-time.After(refresh)成功执行了，那么这条case就不会被执行；
			// 当db.quitChan里面装载了chan error数据的时候，也就是Close函数执行的时候，加了一个chan error到db.quitChan，那么errc = <-db.quitChan就会成功执行，
			// errc就会被赋值为db.quitChan里装载的chan error，那么下一次for循环的时候就不是nil了，就会终止循环
		case <-time.After(refresh):
			// Timeout, gather a new set of stats
			// 等待refresh指定的时间，然后再进行下一轮for循环
		}
	}

	if errc == nil {
		// 代码能走到这里只有一种可能：errc是nil，说明用户还没有执行Close函数，db.quitChan一直没有装载chan error数据，所以errc也是nil；同时，merr不是nil，
		// 也就是上面的for循环里面出了错误，所以就跳出了for循环。出现这种情况的时候，就等待Close函数被执行，往db.quitChan里面装载chan error数据，然后把数据
		// 发送给errc，然后errc就初始化为了一个没有任何数据的chan error；如果一直不调用Close函数，那么errc = <-db.quitChan就一直阻塞，直到Close函数被调用
		errc = <-db.quitChan
	}
	// 这个地方有可能merr不是nil（在for循环中出现了错误），也有可能merr就是nil（for循环中没有出错）。不管merr是否为nil，都执行这条语句。
	// 但是这条语句肯定会被阻塞，因为errc这个channel只被用来接收数据，没有发送数据出去，所以肯定阻塞。但是由于meter是由一条goroutine执行的，所以
	// 等Close调用之后，主线程结束之后，meter这条goroutine也会退出，并且不会报错。但是这样的话显得errc <- merr这一条语句毫无意义。
	// TODO 这条语句冗余了，完全没有必要，是否可以考虑删除掉
	errc <- merr
}

func (db *LDBDatabase) NewBatch() Batch {
	return &ldbBatch{db: db.db, b: new(leveldb.Batch)}
}

// ldbBatch实现了Batch接口定义的每个函数
// ldbBatch的属性包含了第三方goleveldb里面的*leveldb.DB，*leveldb.Batch
type ldbBatch struct {
	db   *leveldb.DB
	b    *leveldb.Batch
	size int
}

func (b *ldbBatch) Put(key, value []byte) error {
	b.b.Put(key, value)
	b.size += len(value) // 增加或者修改数据的时候batch的size增加value的字节数
	return nil
}

func (b *ldbBatch) Delete(key []byte) error {
	b.b.Delete(key)
	b.size += 1 // 删除数据的时候batch的size增加固定值1
	return nil
}

func (b *ldbBatch) Write() error {
	return b.db.Write(b.b, nil)
}

func (b *ldbBatch) ValueSize() int {
	return b.size
}

func (b *ldbBatch) Reset() {
	b.b.Reset()
	b.size = 0
}