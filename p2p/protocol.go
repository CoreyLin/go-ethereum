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

package p2p

import (
	"fmt"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
)

// Protocol represents a P2P subprotocol implementation.
// 代表一个P2P子协议实现
type Protocol struct {
	// Name should contain the official protocol name,
	// often a three-letter word.
	// 应该包含官方协议名称，往往是一个三个字母的单词
	Name string

	// Version should contain the version number of the protocol.
	// 协议的版本
	Version uint

	// Length should contain the number of message codes used
	// by the protocol.
	// 包含被协议用到的消息代码的数量
	Length uint64

	// Run is called in a new goroutine when the protocol has been
	// negotiated with a peer. It should read and write messages from
	// rw. The Payload for each message must be fully consumed.
	//
	// The peer connection is closed when Start returns. It should return
	// any protocol-level error (such as an I/O error) that is
	// encountered.
	// 当协议被和一个对等节点协商的时候Run在一个新的goroutine里被调用。它应该从rw读取和写入消息。
	// 每条消息的负载必须被完全消费。
	// 当Start返回时，对等节点的连接被关闭。它应该返回碰到的任何协议级别的错误（比如I/O错误）
	Run func(peer *Peer, rw MsgReadWriter) error

	// NodeInfo is an optional helper method to retrieve protocol specific metadata
	// about the host node.
	// 是一个用于获取关于主机节点的协议特定的元数据的可选的帮助方法
	NodeInfo func() interface{}

	// PeerInfo is an optional helper method to retrieve protocol specific metadata
	// about a certain peer in the network. If an info retrieval function is set,
	// but returns nil, it is assumed that the protocol handshake is still running.
	// 是一个用于获取关于某个对等节点的协议特定的元数据的可选的帮助方法。如果一个信息获取函数被设置，但是返回nil，
	// 那么我们就认为协议握手还在进行中。
	PeerInfo func(id enode.ID) interface{}

	// Attributes contains protocol specific information for the node record.
	// Attributes包含节点记录的协议特定信息。
	Attributes []enr.Entry
}

func (p Protocol) cap() Cap {
	return Cap{p.Name, p.Version}
}

// Cap is the structure of a peer capability.
type Cap struct {
	Name    string
	Version uint
}

func (cap Cap) String() string {
	return fmt.Sprintf("%s/%d", cap.Name, cap.Version)
}

type capsByNameAndVersion []Cap

func (cs capsByNameAndVersion) Len() int      { return len(cs) }
func (cs capsByNameAndVersion) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }
func (cs capsByNameAndVersion) Less(i, j int) bool {
	return cs[i].Name < cs[j].Name || (cs[i].Name == cs[j].Name && cs[i].Version < cs[j].Version)
}

func (capsByNameAndVersion) ENRKey() string { return "cap" }
