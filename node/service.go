// Copyright 2015 The go-ethereum Authors
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
	"reflect"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rpc"
)

// ServiceContext is a collection of service independent options inherited from
// the protocol stack, that is passed to all constructors to be optionally used;
// as well as utility methods to operate on the service environment.
// ServiceContext是从协议栈继承的服务独立选项的集合，它被传递给所有的构造函数可选地使用;以及在服务环境上操作的实用方法。
type ServiceContext struct {
	// 注意，这个config是和Node（节点）相关的config
	config         *Config
	// 已构建服务的索引
	services       map[reflect.Type]Service // Index of the already constructed services
	// 用于解耦通知的事件多路复用器
	EventMux       *event.TypeMux           // Event multiplexer used for decoupled notifications
	// 由节点创建的账户管理器。
	AccountManager *accounts.Manager        // Account manager created by the node.
}

// OpenDatabase opens an existing database with the given name (or creates one
// if no previous can be found) from within the node's data directory. If the
// node is an ephemeral one, a memory database is returned.
// OpenDatabase从节点的数据目录中打开具有给定名称的现有数据库（如果找不到，则创建一个）。如果节点是短暂的节点，则返回内存数据库。
func (ctx *ServiceContext) OpenDatabase(name string, cache int, handles int) (ethdb.Database, error) {
	if ctx.config.DataDir == "" {
		return ethdb.NewMemDatabase(), nil
	}
	db, err := ethdb.NewLDBDatabase(ctx.config.ResolvePath(name), cache, handles)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// ResolvePath resolves a user path into the data directory if that was relative
// and if the user actually uses persistent storage. It will return an empty string
// for emphemeral storage and the user's own input for absolute paths.
func (ctx *ServiceContext) ResolvePath(path string) string {
	return ctx.config.ResolvePath(path)
}

// Service retrieves a currently running service registered of a specific type.
func (ctx *ServiceContext) Service(service interface{}) error {
	element := reflect.ValueOf(service).Elem()
	if running, ok := ctx.services[element.Type()]; ok {
		element.Set(reflect.ValueOf(running))
		return nil
	}
	return ErrServiceUnknown
}

// ServiceConstructor is the function signature of the constructors needed to be
// registered for service instantiation.
type ServiceConstructor func(ctx *ServiceContext) (Service, error)

// Service is an individual protocol that can be registered into a node.
//
// Notes:
//
// • Service life-cycle management is delegated to the node. The service is allowed to
// initialize itself upon creation, but no goroutines should be spun up outside of the
// Start method.
//
// • Restart logic is not required as the node will create a fresh instance
// every time a service is started.
// 服务是可以注册到节点的单个协议。
// 注：
// • 服务生命周期管理委派给节点。允许服务在创建时自行初始化，但是不应该在Start方法之外执行goroutine。
// • 不需要重新启动逻辑，因为每次启动服务时节点都会创建一个新实例。
type Service interface {
	// Protocols retrieves the P2P protocols the service wishes to start.
	// Protocols检索服务希望启动的P2P协议。
	Protocols() []p2p.Protocol

	// APIs retrieves the list of RPC descriptors the service provides
	// APIs检索服务提供的RPC描述符列表
	APIs() []rpc.API

	// Start is called after all services have been constructed and the networking
	// layer was also initialized to spawn any goroutines required by the service.
	// 在构建所有服务之后调用Start，并且还初始化网络层以生成服务所需的任何goroutine。
	Start(server *p2p.Server) error

	// Stop terminates all goroutines belonging to the service, blocking until they
	// are all terminated.
	// Stop终止属于该服务的所有goroutine，阻塞直到它们全部终止。
	Stop() error
}
