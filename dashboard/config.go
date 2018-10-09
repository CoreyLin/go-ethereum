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

package dashboard

import "time"

// DefaultConfig contains default settings for the dashboard.
// 包含仪表盘的默认设置
var DefaultConfig = Config{
	Host:    "localhost",
	Port:    8080,
	Refresh: 5 * time.Second,
}

// Config contains the configuration parameters of the dashboard.
// 包含仪表盘的配置参数
type Config struct {
	// Host is the host interface on which to start the dashboard server. If this
	// field is empty, no dashboard will be started.
	// 用于启动仪表盘服务器的主机接口。如果为空，仪表盘不会被启动。
	Host string `toml:",omitempty"`

	// Port is the TCP port number on which to start the dashboard server. The
	// default zero value is/ valid and will pick a port number randomly (useful
	// for ephemeral nodes).
	// 用于启动仪表盘服务器的TCP端口号。默认零值是/，是合法的，会随机选择一个端口号（对于短暂的节点很有用）
	Port int `toml:",omitempty"`

	// Refresh is the refresh rate of the data updates, the chartEntry will be collected this often.
	// 数据更新的刷新时间和频率，chartEntry会被经常收集。
	Refresh time.Duration `toml:",omitempty"`
}
