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
	"errors"
	"fmt"
)

const (
	errInvalidMsgCode = iota
	errInvalidMsg
)

var errorToString = map[int]string{
	errInvalidMsgCode: "invalid message code",
	errInvalidMsg:     "invalid message",
}

type peerError struct {
	code    int
	message string
}

func newPeerError(code int, format string, v ...interface{}) *peerError {
	desc, ok := errorToString[code]
	if !ok {
		panic("invalid error code")
	}
	err := &peerError{code, desc}
	if format != "" {
		err.message += ": " + fmt.Sprintf(format, v...)
	}
	return err
}

func (pe *peerError) Error() string {
	return pe.message
}

var errProtocolReturned = errors.New("protocol returned")

type DiscReason uint

const (
	DiscRequested DiscReason = iota
	DiscNetworkError
	DiscProtocolError
	DiscUselessPeer
	DiscTooManyPeers
	DiscAlreadyConnected
	DiscIncompatibleVersion
	DiscInvalidIdentity
	DiscQuitting
	DiscUnexpectedIdentity
	DiscSelf
	DiscReadTimeout
	DiscSubprotocolError = 0x10
)

var discReasonToString = [...]string{
	// 请求断开连接
	DiscRequested:           "disconnect requested",
	// 网络错误
	DiscNetworkError:        "network error",
	// 违反协议
	DiscProtocolError:       "breach of protocol",
	// 没用的peer
	DiscUselessPeer:         "useless peer",
	// 太多peer
	DiscTooManyPeers:        "too many peers",
	// 已经连接了
	DiscAlreadyConnected:    "already connected",
	// 不兼容的p2p协议版本
	DiscIncompatibleVersion: "incompatible p2p protocol version",
	// 无效的节点标识
	DiscInvalidIdentity:     "invalid node identity",
	// 客户退出
	DiscQuitting:            "client quitting",
	// 不期望的身份
	DiscUnexpectedIdentity:  "unexpected identity",
	// 连接到自己
	DiscSelf:                "connected to self",
	// 读取超时
	DiscReadTimeout:         "read timeout",
	// 子协议错误
	DiscSubprotocolError:    "subprotocol error",
}

func (d DiscReason) String() string {
	if len(discReasonToString) < int(d) {
		return fmt.Sprintf("unknown disconnect reason %d", d)
	}
	return discReasonToString[d]
}

func (d DiscReason) Error() string {
	return d.String()
}

func discReasonForError(err error) DiscReason {
	if reason, ok := err.(DiscReason); ok {
		return reason
	}
	if err == errProtocolReturned {
		return DiscQuitting
	}
	peerError, ok := err.(*peerError)
	if ok {
		switch peerError.code {
		case errInvalidMsgCode, errInvalidMsg:
			return DiscProtocolError
		default:
			return DiscSubprotocolError
		}
	}
	return DiscSubprotocolError
}
