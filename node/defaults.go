// Copyright 2016 The go-ethereum Authors
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

package node

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	// HTTP RPC服务器默认主机接口
	DefaultHTTPHost = "localhost" // Default host interface for the HTTP RPC server
	// HTTP RPC服务器默认端口
	DefaultHTTPPort = 8545        // Default TCP port for the HTTP RPC server
	// websocket RPC服务器默认主机接口
	DefaultWSHost   = "localhost" // Default host interface for the websocket RPC server
	// websocket RPC服务器默认端口
	DefaultWSPort   = 8546        // Default TCP port for the websocket RPC server
)

// DefaultConfig contains reasonable default settings.
// 包含合理的默认设置
var DefaultConfig = Config{
	DataDir:          DefaultDataDir(),
	HTTPPort:         DefaultHTTPPort,
	HTTPModules:      []string{"net", "web3"},
	HTTPVirtualHosts: []string{"localhost"},
	HTTPTimeouts:     rpc.DefaultHTTPTimeouts,
	WSPort:           DefaultWSPort,
	WSModules:        []string{"net", "web3"},
	P2P: p2p.Config{
		ListenAddr: ":30303",
		MaxPeers:   25,
		NAT:        nat.Any(),
	},
}

// DefaultDataDir is the default data directory to use for the databases and other
// persistence requirements.
// 获取默认的数据文件夹，用于数据库和其他持久化需求
func DefaultDataDir() string {
	// Try to place the data folder in the user's home dir
	home := homeDir()
	if home != "" {
		// runtime.GOOS代表当前的操作系统
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Ethereum")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Ethereum")
		} else {
			return filepath.Join(home, ".ethereum")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	// 如果不能获取一个稳定的默认数据文件夹路径，就返回空字符串
	return ""
}

func homeDir() string {
	// 获取环境变量HOME的值，如果不为空，就直接返回
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	// 获取当前的系统用户，然后返回用户的家目录的路径
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
