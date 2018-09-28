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

package ethdb

// Code using batches should try to add this much data to the batch.
// The value was determined empirically.
const IdealBatchSize = 100 * 1024

// Putter wraps the database write operation supported by both batches and regular databases.
// 定义了写操作，包括普通写操作和批量写操作。
type Putter interface {
	Put(key []byte, value []byte) error
}

// Deleter wraps the database delete operation supported by both batches and regular databases.
// 定义了删除操作，包括普通删除和批量删除。
type Deleter interface {
	Delete(key []byte) error
}

// Database wraps all database operations. All methods are safe for concurrent use.
// 数据库接口，定义了以太坊里的所有数据库操作，所有操作都是线程安全的。
type Database interface {
	Putter
	Deleter
	Get(key []byte) ([]byte, error) // 获取操作，根据键获取值
	Has(key []byte) (bool, error) // 判断操作，判断键是否在数据库里面存在
	Close() // 关闭操作
	NewBatch() Batch // 这个函数返回一个批处理接口
}

// Batch is a write-only database that commits changes to its host database
// when Write is called. Batch cannot be used concurrently.
// 批量写操作接口，可用于添加，修改和删除数据，当ｗｉｒｔｅ函数被调用时，向数据库写入更改。不能并行使用，线程不安全。
type Batch interface {
	Putter
	Deleter
	ValueSize() int // amount of data in the batch 代表batch里的数据总数，以字节为单位
	Write() error
	// Reset resets the batch for reuse 重置这个批处理
	Reset()
}
