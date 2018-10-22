// Copyright 2017 The go-ethereum Authors
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

package eth

import (
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/params"
)

// DefaultConfig contains default settings for use on the Ethereum main net.
// DefaultConfig包含了用于以太坊主网的默认配置
var DefaultConfig = Config{
	SyncMode: downloader.FastSync,
	// PoW验证模式没有设置，默认是ModeNormal
	Ethash: ethash.Config{
		CacheDir:       "ethash",
		CachesInMem:    2,
		CachesOnDisk:   3,
		DatasetsInMem:  1,
		DatasetsOnDisk: 2,
	},
	NetworkId:     1,
	LightPeers:    100,
	DatabaseCache: 768,
	TrieCache:     256,
	TrieTimeout:   60 * time.Minute,
	MinerGasFloor: 8000000,
	MinerGasCeil:  8000000,
	MinerGasPrice: big.NewInt(params.GWei),
	MinerRecommit: 3 * time.Second,

	TxPool: core.DefaultTxPoolConfig,
	GPO: gasprice.Config{
		Blocks:     20,
		Percentile: 60,
	},
}

func init() {
	home := os.Getenv("HOME")
	if home == "" {
		if user, err := user.Current(); err == nil {
			home = user.HomeDir
		}
	}
	if runtime.GOOS == "windows" {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, "AppData", "Ethash")
	} else {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, ".ethash")
	}
}

//go:generate gencodec -type Config -field-override configMarshaling -formats toml -out gen_config.go

type Config struct {
	// The genesis block, which is inserted if the database is empty.
	// If nil, the Ethereum main net block is used.
	// 如果数据库为空，就插入这个创世区块。如果是nil，就使用以太坊主网区块
	Genesis *core.Genesis `toml:",omitempty"`

	// Protocol options
	// 协议可选项
	// NetworkId用于选择要连接到的对等节点
	NetworkId uint64 // Network ID to use for selecting peers to connect to
	SyncMode  downloader.SyncMode
	NoPruning bool

	// Light client options
	// 轻客户端可选项
	// 允许服务LES请求的时间的最大百分比
	LightServ  int `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	// LES客户端对等节点的最大数量
	LightPeers int `toml:",omitempty"` // Maximum number of LES client peers

	// Database options
	// 数据库可选项
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	// 单位是MB
	DatabaseCache      int
	// 单位是MB
	TrieCache          int
	TrieTimeout        time.Duration

	// Mining-related options
	// 挖矿相关的可选项
	Etherbase      common.Address `toml:",omitempty"`
	MinerNotify    []string       `toml:",omitempty"`
	MinerExtraData []byte         `toml:",omitempty"`
	MinerGasFloor  uint64
	MinerGasCeil   uint64
	MinerGasPrice  *big.Int
	MinerRecommit  time.Duration
	MinerNoverify  bool

	// Ethash options
	// Ethash POW算法可选项
	Ethash ethash.Config

	// Transaction pool options
	// 交易池可选项
	TxPool core.TxPoolConfig

	// Gas Price Oracle options
	// Gas价格Oracle可选项
	GPO gasprice.Config

	// Enables tracking of SHA3 preimages in the VM
	// 在VM中开启SHA3原像追踪
	EnablePreimageRecording bool

	// Miscellaneous options
	DocRoot string `toml:"-"`

	// Type of the EWASM interpreter ("" for detault)
	// EWASM解释器的类型（默认是""）
	// TODO Corey: for detault改为for default
	EWASMInterpreter string
	// Type of the EVM interpreter ("" for default)
	// EVM解释器的类型（默认为“”）
	EVMInterpreter string
}

type configMarshaling struct {
	MinerExtraData hexutil.Bytes
}
