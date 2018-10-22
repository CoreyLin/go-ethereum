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

package params

// These are network parameters that need to be constant between clients, but
// aren't necessarily consensus related.

const (
	// BloomBitsBlocks is the number of blocks a single bloom bit section vector
	// contains on the server side.
	// BloomBitsBlocks是单个布隆位部分向量在服务器端包含的区块数。
	BloomBitsBlocks uint64 = 4096

	// BloomBitsBlocksClient is the number of blocks a single bloom bit section vector
	// contains on the light client side
	// BloomBitsBlocksClient是单个bloom位部分向量在轻客户端侧包含的区块数
	BloomBitsBlocksClient uint64 = 32768

	// BloomConfirms is the number of confirmation blocks before a bloom section is
	// considered probably final and its rotated bits are calculated.
	// BloomConfirms是在Bloom部分被认为可能是final并且计算其旋转位之前的确认区块的数量。
	BloomConfirms = 256

	// CHTFrequencyClient is the block frequency for creating CHTs on the client side.
	// CHTFrequencyClient是在客户端创建CHT的区块频率。
	CHTFrequencyClient = 32768

	// CHTFrequencyServer is the block frequency for creating CHTs on the server side.
	// Eventually this can be merged back with the client version, but that requires a
	// full database upgrade, so that should be left for a suitable moment.
	CHTFrequencyServer = 4096

	// BloomTrieFrequency is the block frequency for creating BloomTrie on both
	// server/client sides.
	// BloomTrieFrequency是在服务器/客户端创建BloomTrie的块频率。
	BloomTrieFrequency = 32768

	// HelperTrieConfirmations is the number of confirmations before a client is expected
	// to have the given HelperTrie available.
	// HelperTrieConfirmations是客户预期可获得给定HelperTrie之前的确认数。
	HelperTrieConfirmations = 2048

	// HelperTrieProcessConfirmations is the number of confirmations before a HelperTrie
	// is generated
	HelperTrieProcessConfirmations = 256
)
