// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// Package utils contains internal helper functions for go-ethereum commands.
// utils包包含对于go-ethereum命令的内部帮助函数
package utils

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/clique"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/dashboard"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/ethstats"
	"github.com/ethereum/go-ethereum/les"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/metrics/influxdb"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/p2p/netutil"
	"github.com/ethereum/go-ethereum/params"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv6"
	"gopkg.in/urfave/cli.v1"
)

var (
	CommandHelpTemplate = `{{.cmd.Name}}{{if .cmd.Subcommands}} command{{end}}{{if .cmd.Flags}} [command options]{{end}} [arguments...]
{{if .cmd.Description}}{{.cmd.Description}}
{{end}}{{if .cmd.Subcommands}}
SUBCOMMANDS:
	{{range .cmd.Subcommands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}
	{{end}}{{end}}{{if .categorizedFlags}}
{{range $idx, $categorized := .categorizedFlags}}{{$categorized.Name}} OPTIONS:
{{range $categorized.Flags}}{{"\t"}}{{.}}
{{end}}
{{end}}{{end}}`
)

func init() {
	cli.AppHelpTemplate = `{{.Name}} {{if .Flags}}[global options] {{end}}command{{if .Flags}} [command options]{{end}} [arguments...]

VERSION:
   {{.Version}}

COMMANDS:
   {{range .Commands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}
   {{end}}{{if .Flags}}
GLOBAL OPTIONS:
   {{range .Flags}}{{.}}
   {{end}}{{end}}
`

	cli.CommandHelpTemplate = CommandHelpTemplate
}

// NewApp creates an app with sane defaults.
func NewApp(gitCommit, usage string) *cli.App {
	// 用合理的Name, Usage, Version，Action的默认值创建一个新的cli程序
	app := cli.NewApp()
	// 注意：os.Args[0]代表的是程序的绝对路径，那么filepath.Base(os.Args[0])代表的是程序名
	app.Name = filepath.Base(os.Args[0])
	// 注意：Use App.Authors, this is deprecated
	app.Author = ""
	//app.Authors = nil
	app.Email = ""
	// 获取Version，比如1.8.16-unstable
	app.Version = params.VersionWithMeta
	// 如果gitCommit的长度大于等于8，那么把gitCommit的前8个字符追加到Version的后面
	if len(gitCommit) >= 8 {
		app.Version += "-" + gitCommit[:8]
	}
	app.Usage = usage
	return app
}

// These are all the command line flags we support.
// If you add to this list, please remember to include the
// flag in the appropriate command definition.
//
// The flags are defined here so their names and help texts
// are the same for all commands.

var (
	// General settings
	DataDirFlag = DirectoryFlag{
		Name:  "datadir",
		Usage: "Data directory for the databases and keystore",
		// 获取了数据默认的文件夹路径，也就是说无论如何都有一个默认路径
		Value: DirectoryString{node.DefaultDataDir()},
	}
	KeyStoreDirFlag = DirectoryFlag{
		Name:  "keystore",
		Usage: "Directory for the keystore (default = inside the datadir)",
	}
	NoUSBFlag = cli.BoolFlag{
		Name:  "nousb",
		Usage: "Disables monitoring for and managing USB hardware wallets",
	}
	NetworkIdFlag = cli.Uint64Flag{
		Name:  "networkid",
		// 网络标识符（整数，1 = Frontier，2 = Morden（废弃），3 = Ropsten，4 = Rinkeby），默认是1
		Usage: "Network identifier (integer, 1=Frontier, 2=Morden (disused), 3=Ropsten, 4=Rinkeby)",
		Value: eth.DefaultConfig.NetworkId,
	}
	TestnetFlag = cli.BoolFlag{
		Name:  "testnet",
		// Ropsten网络：预先配置的工作量证明测试网络
		Usage: "Ropsten network: pre-configured proof-of-work test network",
	}
	RinkebyFlag = cli.BoolFlag{
		Name:  "rinkeby",
		// Rinkeby网络：预先配置的权威证明测试网络
		Usage: "Rinkeby network: pre-configured proof-of-authority test network",
	}
	DeveloperFlag = cli.BoolFlag{
		Name:  "dev",
		// 短暂的权威证明网络与预先资助的开发者帐户，启用了挖矿
		Usage: "Ephemeral proof-of-authority network with a pre-funded developer account, mining enabled",
	}
	DeveloperPeriodFlag = cli.IntFlag{
		Name:  "dev.period",
		// 在开发者模式下使用的区块时间段（0 =仅当交易处理暂挂时挖矿）
		Usage: "Block period to use in developer mode (0 = mine only if transaction pending)",
	}
	IdentityFlag = cli.StringFlag{
		Name:  "identity",
		Usage: "Custom node name",
	}
	DocRootFlag = DirectoryFlag{
		Name:  "docroot",
		// HTTPClient文件方案的文档根目录，默认是用户家目录
		Usage: "Document Root for HTTPClient file scheme",
		Value: DirectoryString{homeDir()},
	}
	defaultSyncMode = eth.DefaultConfig.SyncMode
	SyncModeFlag    = TextMarshalerFlag{
		Name:  "syncmode",
		Usage: `Blockchain sync mode ("fast", "full", or "light")`,
		Value: &defaultSyncMode,
	}
	GCModeFlag = cli.StringFlag{
		Name:  "gcmode",
		// 区块链垃圾收集模式（“完整”，“存档”）
		Usage: `Blockchain garbage collection mode ("full", "archive")`,
		Value: "full",
	}
	LightServFlag = cli.IntFlag{
		Name:  "lightserv",
		// 服务LES请求所允许的最大时间百分比（0-90），默认是0
		Usage: "Maximum percentage of time allowed for serving LES requests (0-90)",
		Value: 0,
	}
	LightPeersFlag = cli.IntFlag{
		Name:  "lightpeers",
		// LES客户端对等体的最大数量，默认是100
		Usage: "Maximum number of LES client peers",
		Value: eth.DefaultConfig.LightPeers,
	}
	LightKDFFlag = cli.BoolFlag{
		Name:  "lightkdf",
		Usage: "Reduce key-derivation RAM & CPU usage at some expense of KDF strength",
	}
	// Dashboard settings
	// 仪表盘设置
	DashboardEnabledFlag = cli.BoolFlag{
		Name:  metrics.DashboardEnabledFlag,
		Usage: "Enable the dashboard",
	}
	DashboardAddrFlag = cli.StringFlag{
		Name:  "dashboard.addr",
		// 仪表板监听接口，默认localhost
		Usage: "Dashboard listening interface",
		Value: dashboard.DefaultConfig.Host,
	}
	DashboardPortFlag = cli.IntFlag{
		Name:  "dashboard.host",
		// 仪表板监听端口，默认8080
		Usage: "Dashboard listening port",
		Value: dashboard.DefaultConfig.Port,
	}
	DashboardRefreshFlag = cli.DurationFlag{
		Name:  "dashboard.refresh",
		// 仪表板指标集合刷新率，默认5秒
		Usage: "Dashboard metrics collection refresh rate",
		Value: dashboard.DefaultConfig.Refresh,
	}
	// Ethash settings
	EthashCacheDirFlag = DirectoryFlag{
		Name:  "ethash.cachedir",
		// 存储ethash验证缓存的目录（默认=在datadir内）
		Usage: "Directory to store the ethash verification caches (default = inside the datadir)",
	}
	EthashCachesInMemoryFlag = cli.IntFlag{
		Name:  "ethash.cachesinmem",
		// 最近保留在内存中的ethash缓存数量（每个16MB），默认是2
		Usage: "Number of recent ethash caches to keep in memory (16MB each)",
		Value: eth.DefaultConfig.Ethash.CachesInMem,
	}
	EthashCachesOnDiskFlag = cli.IntFlag{
		Name:  "ethash.cachesondisk",
		// 最近保留在磁盘上的ethash缓存数量（每个16MB），默认是3
		Usage: "Number of recent ethash caches to keep on disk (16MB each)",
		Value: eth.DefaultConfig.Ethash.CachesOnDisk,
	}
	EthashDatasetDirFlag = DirectoryFlag{
		Name:  "ethash.dagdir",
		// 用于存储ethash挖矿DAG的目录（默认=在用户家目录内）
		Usage: "Directory to store the ethash mining DAGs (default = inside home folder)",
		Value: DirectoryString{eth.DefaultConfig.Ethash.DatasetDir},
	}
	EthashDatasetsInMemoryFlag = cli.IntFlag{
		Name:  "ethash.dagsinmem",
		// 最近的ethash挖矿DAG保留在内存中的数量（每个1 + GB），默认是1
		Usage: "Number of recent ethash mining DAGs to keep in memory (1+GB each)",
		Value: eth.DefaultConfig.Ethash.DatasetsInMem,
	}
	EthashDatasetsOnDiskFlag = cli.IntFlag{
		Name:  "ethash.dagsondisk",
		// 最近保留在磁盘上的ethash挖矿DAG的数量（每个1 + GB），默认是2
		Usage: "Number of recent ethash mining DAGs to keep on disk (1+GB each)",
		Value: eth.DefaultConfig.Ethash.DatasetsOnDisk,
	}
	// Transaction pool settings
	// 交易池设置
	TxPoolLocalsFlag = cli.StringFlag{
		Name:  "txpool.locals",
		// 被视为locals的逗号分隔的帐户（没有flush，优先包含）
		Usage: "Comma separated accounts to treat as locals (no flush, priority inclusion)",
	}
	TxPoolNoLocalsFlag = cli.BoolFlag{
		Name:  "txpool.nolocals",
		// 禁用本地提交的交易的价格豁免
		Usage: "Disables price exemptions for locally submitted transactions",
	}
	TxPoolJournalFlag = cli.StringFlag{
		Name:  "txpool.journal",
		// 用于本地交易的磁盘日志以幸免于节点重新启动
		Usage: "Disk journal for local transaction to survive node restarts",
		Value: core.DefaultTxPoolConfig.Journal,
	}
	TxPoolRejournalFlag = cli.DurationFlag{
		Name:  "txpool.rejournal",
		// 重新生成本地交易日志的时间间隔，默认是1小时
		Usage: "Time interval to regenerate the local transaction journal",
		Value: core.DefaultTxPoolConfig.Rejournal,
	}
	TxPoolPriceLimitFlag = cli.Uint64Flag{
		Name:  "txpool.pricelimit",
		// 实施接纳进交易池的最低gas价格限制，默认是1wei
		Usage: "Minimum gas price limit to enforce for acceptance into the pool",
		Value: eth.DefaultConfig.TxPool.PriceLimit,
	}
	TxPoolPriceBumpFlag = cli.Uint64Flag{
		Name:  "txpool.pricebump",
		// 用于替换一笔现有交易的价格暴涨百分比，默认是10
		Usage: "Price bump percentage to replace an already existing transaction",
		Value: eth.DefaultConfig.TxPool.PriceBump,
	}
	TxPoolAccountSlotsFlag = cli.Uint64Flag{
		Name:  "txpool.accountslots",
		// 每个帐户保证的最小可执行交易处理槽数，默认是16
		Usage: "Minimum number of executable transaction slots guaranteed per account",
		Value: eth.DefaultConfig.TxPool.AccountSlots,
	}
	TxPoolGlobalSlotsFlag = cli.Uint64Flag{
		Name:  "txpool.globalslots",
		// 所有帐户的最大可执行交易槽数，默认是4096
		Usage: "Maximum number of executable transaction slots for all accounts",
		Value: eth.DefaultConfig.TxPool.GlobalSlots,
	}
	TxPoolAccountQueueFlag = cli.Uint64Flag{
		Name:  "txpool.accountqueue",
		// 每个帐户允许的最大非可执行交易槽数，也就是队列里排队的交易数，默认是64
		Usage: "Maximum number of non-executable transaction slots permitted per account",
		Value: eth.DefaultConfig.TxPool.AccountQueue,
	}
	TxPoolGlobalQueueFlag = cli.Uint64Flag{
		Name:  "txpool.globalqueue",
		// 所有帐户的最大不可执行交易槽数，默认是1024
		Usage: "Maximum number of non-executable transaction slots for all accounts",
		Value: eth.DefaultConfig.TxPool.GlobalQueue,
	}
	TxPoolLifetimeFlag = cli.DurationFlag{
		Name:  "txpool.lifetime",
		// 非可执行交易排队的最长时间，默认是3小时
		Usage: "Maximum amount of time non-executable transaction are queued",
		Value: eth.DefaultConfig.TxPool.Lifetime,
	}
	// Performance tuning settings
	// 性能调整设置
	CacheFlag = cli.IntFlag{
		Name:  "cache",
		// 分配给内部缓存的兆字节内存，默认是1024M
		Usage: "Megabytes of memory allocated to internal caching",
		Value: 1024,
	}
	CacheDatabaseFlag = cli.IntFlag{
		Name:  "cache.database",
		// 用于数据库io的缓存内存容量百分比，默认75
		Usage: "Percentage of cache memory allowance to use for database io",
		Value: 50,
	}
	CacheTrieFlag = cli.IntFlag{
		Name:  "cache.trie",
		Usage: "Percentage of cache memory allowance to use for trie caching",
		Value: 25,
	}
	CacheGCFlag = cli.IntFlag{
		Name:  "cache.gc",
		// 用于trie修剪的缓存内存容量百分比
		Usage: "Percentage of cache memory allowance to use for trie pruning",
		Value: 25,
	}
	TrieCacheGenFlag = cli.IntFlag{
		Name:  "trie-cache-gens",
		// 要保留在内存中的trie节点生成数
		Usage: "Number of trie node generations to keep in memory",
		Value: int(state.MaxTrieCacheGen),
	}
	// Miner settings
	MiningEnabledFlag = cli.BoolFlag{
		Name:  "mine",
		Usage: "Enable mining",
	}
	MinerThreadsFlag = cli.IntFlag{
		Name:  "miner.threads",
		Usage: "Number of CPU threads to use for mining",
		Value: 0,
	}
	MinerLegacyThreadsFlag = cli.IntFlag{
		Name:  "minerthreads",
		Usage: "Number of CPU threads to use for mining (deprecated, use --miner.threads)",
		Value: 0,
	}
	MinerNotifyFlag = cli.StringFlag{
		Name:  "miner.notify",
		// 逗号分隔的HTTP URL列表以通知新的工作包
		Usage: "Comma separated HTTP URL list to notify of new work packages",
	}
	MinerGasTargetFlag = cli.Uint64Flag{
		Name:  "miner.gastarget",
		// 已开采区块的目标gas的下限，默认8000000
		Usage: "Target gas floor for mined blocks",
		Value: eth.DefaultConfig.MinerGasFloor,
	}
	MinerLegacyGasTargetFlag = cli.Uint64Flag{
		Name:  "targetgaslimit",
		// 已开采区块的目标gas的下限（不建议使用，使用--miner.gastarget），默认1e9wei
		Usage: "Target gas floor for mined blocks (deprecated, use --miner.gastarget)",
		Value: eth.DefaultConfig.MinerGasFloor,
	}
	MinerGasLimitFlag = cli.Uint64Flag{
		Name:  "miner.gaslimit",
		// 采矿区块的目标gas的上限，默认8000000
		Usage: "Target gas ceiling for mined blocks",
		Value: eth.DefaultConfig.MinerGasCeil,
	}
	MinerGasPriceFlag = BigFlag{
		Name:  "miner.gasprice",
		// 采矿交易的最低gas价格，默认1GWei
		Usage: "Minimum gas price for mining a transaction",
		Value: eth.DefaultConfig.MinerGasPrice,
	}
	MinerLegacyGasPriceFlag = BigFlag{
		Name:  "gasprice",
		// 采矿交易的最低gas价格（不建议使用，使用--miner.gasprice），默认1GWei
		Usage: "Minimum gas price for mining a transaction (deprecated, use --miner.gasprice)",
		Value: eth.DefaultConfig.MinerGasPrice,
	}
	MinerEtherbaseFlag = cli.StringFlag{
		Name:  "miner.etherbase",
		// 区块挖矿奖励的公共地址（默认=第一个帐户）
		Usage: "Public address for block mining rewards (default = first account)",
		Value: "0",
	}
	MinerLegacyEtherbaseFlag = cli.StringFlag{
		Name:  "etherbase",
		// 区块挖矿奖励的公共地址（默认=第一个帐户，不建议使用，使用--miner.etherbase）
		Usage: "Public address for block mining rewards (default = first account, deprecated, use --miner.etherbase)",
		Value: "0",
	}
	MinerExtraDataFlag = cli.StringFlag{
		Name:  "miner.extradata",
		// 矿工设置的区块额外数据（默认=客户端版本）
		Usage: "Block extra data set by the miner (default = client version)",
	}
	MinerLegacyExtraDataFlag = cli.StringFlag{
		Name:  "extradata",
		// 矿工设置的区块额外数据（默认=客户端版本，不推荐使用，使用--miner.extradata）
		Usage: "Block extra data set by the miner (default = client version, deprecated, use --miner.extradata)",
	}
	MinerRecommitIntervalFlag = cli.DurationFlag{
		Name:  "miner.recommit",
		// 重新创建正在被挖掘的区块的时间间隔，默认3秒
		Usage: "Time interval to recreate the block being mined",
		Value: eth.DefaultConfig.MinerRecommit,
	}
	MinerNoVerfiyFlag = cli.BoolFlag{
		Name:  "miner.noverify",
		// 禁用远程密封验证
		Usage: "Disable remote sealing verification",
	}
	// Account settings
	UnlockedAccountFlag = cli.StringFlag{
		Name:  "unlock",
		Usage: "Comma separated list of accounts to unlock",
		Value: "",
	}
	PasswordFileFlag = cli.StringFlag{
		Name:  "password",
		Usage: "Password file to use for non-interactive password input",
		Value: "",
	}

	VMEnableDebugFlag = cli.BoolFlag{
		Name:  "vmdebug",
		// 记录对VM和合约调试有用的信息
		Usage: "Record information useful for VM and contract debugging",
	}
	// Logging and debug settings
	// 日志和调试设置
	EthStatsURLFlag = cli.StringFlag{
		Name:  "ethstats",
		// ethstats服务的报告URL（节点名：secret@host:port）
		Usage: "Reporting URL of a ethstats service (nodename:secret@host:port)",
	}
	FakePoWFlag = cli.BoolFlag{
		Name:  "fakepow",
		Usage: "Disables proof-of-work verification",
	}
	NoCompactionFlag = cli.BoolFlag{
		Name:  "nocompaction",
		Usage: "Disables db compaction after import",
	}
	// RPC settings
	RPCEnabledFlag = cli.BoolFlag{
		Name:  "rpc",
		Usage: "Enable the HTTP-RPC server",
	}
	RPCListenAddrFlag = cli.StringFlag{
		Name:  "rpcaddr",
		Usage: "HTTP-RPC server listening interface",
		Value: node.DefaultHTTPHost,
	}
	RPCPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
		Value: node.DefaultHTTPPort,
	}
	RPCCORSDomainFlag = cli.StringFlag{
		Name:  "rpccorsdomain",
		Usage: "Comma separated list of domains from which to accept cross origin requests (browser enforced)",
		Value: "",
	}
	RPCVirtualHostsFlag = cli.StringFlag{
		Name:  "rpcvhosts",
		Usage: "Comma separated list of virtual hostnames from which to accept requests (server enforced). Accepts '*' wildcard.",
		Value: strings.Join(node.DefaultConfig.HTTPVirtualHosts, ","),
	}
	RPCApiFlag = cli.StringFlag{
		Name:  "rpcapi",
		Usage: "API's offered over the HTTP-RPC interface",
		Value: "",
	}
	IPCDisabledFlag = cli.BoolFlag{
		Name:  "ipcdisable",
		Usage: "Disable the IPC-RPC server",
	}
	IPCPathFlag = DirectoryFlag{
		Name:  "ipcpath",
		Usage: "Filename for IPC socket/pipe within the datadir (explicit paths escape it)",
	}
	WSEnabledFlag = cli.BoolFlag{
		Name:  "ws",
		Usage: "Enable the WS-RPC server",
	}
	WSListenAddrFlag = cli.StringFlag{
		Name:  "wsaddr",
		Usage: "WS-RPC server listening interface",
		Value: node.DefaultWSHost,
	}
	WSPortFlag = cli.IntFlag{
		Name:  "wsport",
		Usage: "WS-RPC server listening port",
		Value: node.DefaultWSPort,
	}
	WSApiFlag = cli.StringFlag{
		Name:  "wsapi",
		Usage: "API's offered over the WS-RPC interface",
		Value: "",
	}
	WSAllowedOriginsFlag = cli.StringFlag{
		Name:  "wsorigins",
		Usage: "Origins from which to accept websockets requests",
		Value: "",
	}
	ExecFlag = cli.StringFlag{
		Name:  "exec",
		Usage: "Execute JavaScript statement",
	}
	PreloadJSFlag = cli.StringFlag{
		Name:  "preload",
		Usage: "Comma separated list of JavaScript files to preload into the console",
	}

	// Network Settings
	MaxPeersFlag = cli.IntFlag{
		Name:  "maxpeers",
		Usage: "Maximum number of network peers (network disabled if set to 0)",
		Value: 25,
	}
	MaxPendingPeersFlag = cli.IntFlag{
		Name:  "maxpendpeers",
		Usage: "Maximum number of pending connection attempts (defaults used if set to 0)",
		Value: 0,
	}
	ListenPortFlag = cli.IntFlag{
		Name:  "port",
		Usage: "Network listening port",
		Value: 30303,
	}
	BootnodesFlag = cli.StringFlag{
		Name:  "bootnodes",
		Usage: "Comma separated enode URLs for P2P discovery bootstrap (set v4+v5 instead for light servers)",
		Value: "",
	}
	BootnodesV4Flag = cli.StringFlag{
		Name:  "bootnodesv4",
		Usage: "Comma separated enode URLs for P2P v4 discovery bootstrap (light server, full nodes)",
		Value: "",
	}
	BootnodesV5Flag = cli.StringFlag{
		Name:  "bootnodesv5",
		Usage: "Comma separated enode URLs for P2P v5 discovery bootstrap (light server, light nodes)",
		Value: "",
	}
	NodeKeyFileFlag = cli.StringFlag{
		Name:  "nodekey",
		Usage: "P2P node key file",
	}
	NodeKeyHexFlag = cli.StringFlag{
		Name:  "nodekeyhex",
		Usage: "P2P node key as hex (for testing)",
	}
	NATFlag = cli.StringFlag{
		Name:  "nat",
		Usage: "NAT port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
		Value: "any",
	}
	NoDiscoverFlag = cli.BoolFlag{
		Name:  "nodiscover",
		Usage: "Disables the peer discovery mechanism (manual peer addition)",
	}
	DiscoveryV5Flag = cli.BoolFlag{
		Name:  "v5disc",
		Usage: "Enables the experimental RLPx V5 (Topic Discovery) mechanism",
	}
	NetrestrictFlag = cli.StringFlag{
		Name:  "netrestrict",
		Usage: "Restricts network communication to the given IP networks (CIDR masks)",
	}

	// ATM the url is left to the user and deployment to
	JSpathFlag = cli.StringFlag{
		Name:  "jspath",
		Usage: "JavaScript root path for `loadScript`",
		Value: ".",
	}

	// Gas price oracle settings
	// Gas价格oracle设置
	GpoBlocksFlag = cli.IntFlag{
		Name:  "gpoblocks",
		// 检查gas价格的最近区块数量，默认值是20
		Usage: "Number of recent blocks to check for gas prices",
		Value: eth.DefaultConfig.GPO.Blocks,
	}
	GpoPercentileFlag = cli.IntFlag{
		Name:  "gpopercentile",
		// 建议的gas价格是一组近期交易gas价格的给定百分位数, 默认是60%
		Usage: "Suggested gas price is the given percentile of a set of recent transaction gas prices",
		Value: eth.DefaultConfig.GPO.Percentile,
	}
	WhisperEnabledFlag = cli.BoolFlag{
		Name:  "shh",
		Usage: "Enable Whisper",
	}
	WhisperMaxMessageSizeFlag = cli.IntFlag{
		Name:  "shh.maxmessagesize",
		// 接受的最大消息大小，默认1MB
		Usage: "Max message size accepted",
		Value: int(whisper.DefaultMaxMessageSize),
	}
	WhisperMinPOWFlag = cli.Float64Flag{
		Name:  "shh.pow",
		Usage: "Minimum POW accepted",
		Value: whisper.DefaultMinimumPoW,
	}
	WhisperRestrictConnectionBetweenLightClientsFlag = cli.BoolFlag{
		Name:  "shh.restrict-light",
		// 限制两个whisper轻客户端之间的连接
		Usage: "Restrict connection between two whisper light clients",
	}

	// Metrics flags
	MetricsEnabledFlag = cli.BoolFlag{
		Name:  metrics.MetricsEnabledFlag,
		Usage: "Enable metrics collection and reporting",
	}
	MetricsEnableInfluxDBFlag = cli.BoolFlag{
		Name:  "metrics.influxdb",
		Usage: "Enable metrics export/push to an external InfluxDB database",
	}
	MetricsInfluxDBEndpointFlag = cli.StringFlag{
		Name:  "metrics.influxdb.endpoint",
		Usage: "InfluxDB API endpoint to report metrics to",
		Value: "http://localhost:8086",
	}
	MetricsInfluxDBDatabaseFlag = cli.StringFlag{
		Name:  "metrics.influxdb.database",
		Usage: "InfluxDB database name to push reported metrics to",
		Value: "geth",
	}
	MetricsInfluxDBUsernameFlag = cli.StringFlag{
		Name:  "metrics.influxdb.username",
		Usage: "Username to authorize access to the database",
		Value: "test",
	}
	MetricsInfluxDBPasswordFlag = cli.StringFlag{
		Name:  "metrics.influxdb.password",
		Usage: "Password to authorize access to the database",
		Value: "test",
	}
	// The `host` tag is part of every measurement sent to InfluxDB. Queries on tags are faster in InfluxDB.
	// It is used so that we can group all nodes and average a measurement across all of them, but also so
	// that we can select a specific node and inspect its measurements.
	// https://docs.influxdata.com/influxdb/v1.4/concepts/key_concepts/#tag-key
	MetricsInfluxDBHostTagFlag = cli.StringFlag{
		Name:  "metrics.influxdb.host.tag",
		Usage: "InfluxDB `host` tag attached to all measurements",
		Value: "localhost",
	}

	EWASMInterpreterFlag = cli.StringFlag{
		Name:  "vm.ewasm",
		Usage: "External ewasm configuration (default = built-in interpreter)",
		Value: "",
	}
	EVMInterpreterFlag = cli.StringFlag{
		Name:  "vm.evm",
		Usage: "External EVM configuration (default = built-in interpreter)",
		Value: "",
	}
)

// MakeDataDir retrieves the currently requested data directory, terminating
// if none (or the empty string) is specified. If the node is starting a testnet,
// the a subdirectory of the specified datadir will be used.
func MakeDataDir(ctx *cli.Context) string {
	if path := ctx.GlobalString(DataDirFlag.Name); path != "" {
		if ctx.GlobalBool(TestnetFlag.Name) {
			return filepath.Join(path, "testnet")
		}
		if ctx.GlobalBool(RinkebyFlag.Name) {
			return filepath.Join(path, "rinkeby")
		}
		return path
	}
	Fatalf("Cannot determine default data directory, please set manually (--datadir)")
	return ""
}

// setNodeKey creates a node key from set command line flags, either loading it
// from a file or as a specified hex value. If neither flags were provided, this
// method returns nil and an emphemeral key is to be generated.
// 从命令行输入的flag（nodekeyhex或者nodekey）创建一个节点私钥，既可以从一个私钥文件加载，也可以从一个十六进制值转换而来。
// 如果没有输入相关的flag，那么会生成一个暂时的私钥。
func setNodeKey(ctx *cli.Context, cfg *p2p.Config) {
	var (
		// hex代表nodekeyhex flag，节点的私钥（16进制表示）
		hex  = ctx.GlobalString(NodeKeyHexFlag.Name)
		// file代表nodekey flag，节点的私钥文件
		file = ctx.GlobalString(NodeKeyFileFlag.Name)
		key  *ecdsa.PrivateKey
		err  error
	)
	switch {
	// 如果命令行同时输入了nodekeyhex和nodekey两个flag，那么打印消息到标准错误输出，并且退出程序
	// %q	a double-quoted string safely escaped with Go syntax
	case file != "" && hex != "":
		Fatalf("Options %q and %q are mutually exclusive", NodeKeyFileFlag.Name, NodeKeyHexFlag.Name)
	case file != "":
		// 从给定的节点私钥文件里加载一个secp256k1私钥
		if key, err = crypto.LoadECDSA(file); err != nil {
			// 加载私钥失败就打印消息到标准错误输出并退出
			Fatalf("Option %q: %v", NodeKeyFileFlag.Name, err)
		}
		cfg.PrivateKey = key
	case hex != "":
		// 把十六进制的私钥字符串转换为*ecdsa.PrivateKey类型的私钥
		if key, err = crypto.HexToECDSA(hex); err != nil {
			Fatalf("Option %q: %v", NodeKeyHexFlag.Name, err)
		}
		cfg.PrivateKey = key
	}
}

// setNodeUserIdent creates the user identifier from CLI flags.
// 从命令行flag创建自定义用户标识符
func setNodeUserIdent(ctx *cli.Context, cfg *node.Config) {
	if identity := ctx.GlobalString(IdentityFlag.Name); len(identity) > 0 {
		// 如果identity flag被设置了，并且值不是空字符串，那就把值赋给cfg.UserIdent
		cfg.UserIdent = identity
	}
}

// setBootstrapNodes creates a list of bootstrap nodes from the command line
// flags, reverting to pre-configured ones if none have been specified.
// 根据命令行flag创建一个引导节点的列表，如果没有输入任何相关flag，就使用预配置的flag
func setBootstrapNodes(ctx *cli.Context, cfg *p2p.Config) {
	// 获取默认的不同国家或地区的引导节点
	urls := params.MainnetBootnodes
	switch {
	case ctx.GlobalIsSet(BootnodesFlag.Name) || ctx.GlobalIsSet(BootnodesV4Flag.Name):
		// "bootnodesv4" flag的优先级比"bootnodes" flag的优先级高。
		if ctx.GlobalIsSet(BootnodesV4Flag.Name) {
			urls = strings.Split(ctx.GlobalString(BootnodesV4Flag.Name), ",")
		} else {
			urls = strings.Split(ctx.GlobalString(BootnodesFlag.Name), ",")
		}
	case ctx.GlobalBool(TestnetFlag.Name):
		// 如果"testnet" flag被设置为true，意味着Ropsten测试网络打开，就把Ropsten测试网络的默认引导节点赋值给urls
		// 注：Ropsten使用的是PoW共识机制
		urls = params.TestnetBootnodes
	case ctx.GlobalBool(RinkebyFlag.Name):
		// 如果"rinkeby" flag被设置为true，意味着Rinkeby测试网络打开，就把Rinkeby测试网络的默认引导节点赋值给urls
		// 注：Rinkeby使用的是PoA共识机制
		urls = params.RinkebyBootnodes
	case cfg.BootstrapNodes != nil:
		// 如果上面几种flag都没有设置，并且cfg.BootstrapNodes不为nil，已经有值了，就直接返回
		return // already set, don't apply defaults.
	}

	// 创建一个切片，长度为0，容量为以上urls的长度
	// 能走到这一步，有几种可能：
	// 1.flag bootnodesv4，bootnodes，testnet，rinkeby的其中一个被设置了，不管cfg.BootstrapNodes是否为nil，都走到这一步。
	//		如果cfg.BootstrapNodes的值不为nil，那么就会接下来的重新初始化就会把之前的值冲掉。疑问：以前的值被冲掉是否有问题？
	// 2.flag bootnodesv4，bootnodes，testnet，rinkeby都没有被设置，并且cfg.BootstrapNodes的值为nil，才会走到这一步
	cfg.BootstrapNodes = make([]*enode.Node, 0, len(urls))
	for _, url := range urls {
		// 解析节点标识符，即Node URL
		node, err := enode.ParseV4(url)

		if err != nil {
			// 打印CRIT级别的日志，并且退出程序，退出码为1
			log.Crit("Bootstrap URL invalid", "enode", url, "err", err)
		}
		cfg.BootstrapNodes = append(cfg.BootstrapNodes, node)
	}
}

// setBootstrapNodesV5 creates a list of bootstrap nodes from the command line
// flags, reverting to pre-configured ones if none have been specified.
// 根据命令行flag创建一个引导节点的列表，如果没有输入任何相关flag，就使用预配置的flag
func setBootstrapNodesV5(ctx *cli.Context, cfg *p2p.Config) {
	urls := params.DiscoveryV5Bootnodes
	switch {
	case ctx.GlobalIsSet(BootnodesFlag.Name) || ctx.GlobalIsSet(BootnodesV5Flag.Name):
		// flag bootnodes或者bootnodesv5
		if ctx.GlobalIsSet(BootnodesV5Flag.Name) {
			urls = strings.Split(ctx.GlobalString(BootnodesV5Flag.Name), ",")
		} else {
			urls = strings.Split(ctx.GlobalString(BootnodesFlag.Name), ",")
		}
	case ctx.GlobalBool(RinkebyFlag.Name):
		urls = params.RinkebyBootnodes
	case cfg.BootstrapNodesV5 != nil:
		return // already set, don't apply defaults.
	}

	cfg.BootstrapNodesV5 = make([]*discv5.Node, 0, len(urls))
	for _, url := range urls {
		node, err := discv5.ParseNode(url)
		if err != nil {
			log.Error("Bootstrap URL invalid", "enode", url, "err", err)
			continue
		}
		cfg.BootstrapNodesV5 = append(cfg.BootstrapNodesV5, node)
	}
}

// setListenAddress creates a TCP listening address string from set command
// line flags.
// 从命令行flag创建一个TCP监听端口
func setListenAddress(ctx *cli.Context, cfg *p2p.Config) {
	if ctx.GlobalIsSet(ListenPortFlag.Name) {
		// 如果port flag被输入了，就在前面加一个冒号，并且返回。如果没有输入port flag，默认是30303
		cfg.ListenAddr = fmt.Sprintf(":%d", ctx.GlobalInt(ListenPortFlag.Name))
	}
}

// setNAT creates a port mapper from command line flags.
// 从命令行flag创建一个端口映射器
func setNAT(ctx *cli.Context, cfg *p2p.Config) {
	// 检查nat flag是否被命令行输入，如果没有输入默认值是“any”，any就代表使用第一个自动检测到的机制
	if ctx.GlobalIsSet(NATFlag.Name) {
		natif, err := nat.Parse(ctx.GlobalString(NATFlag.Name))
		if err != nil {
			Fatalf("Option %s: %v", NATFlag.Name, err)
		}
		cfg.NAT = natif
	}
}

// splitAndTrim splits input separated by a comma
// and trims excessive white space from the substrings.
// 分隔用逗号分开的输入，并且删掉多余的空格
func splitAndTrim(input string) []string {
	result := strings.Split(input, ",")
	for i, r := range result {
		result[i] = strings.TrimSpace(r)
	}
	return result
}

// setHTTP creates the HTTP RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
// 从命令行flag创建HTTP RPC监听接口
func setHTTP(ctx *cli.Context, cfg *node.Config) {
	if ctx.GlobalBool(RPCEnabledFlag.Name) && cfg.HTTPHost == "" {
		// 如果rpc flag的值为true，并且cfg.HTTPHost为空，则设置为127.0.0.1
		cfg.HTTPHost = "127.0.0.1"
		if ctx.GlobalIsSet(RPCListenAddrFlag.Name) {
			// 如果rpcaddr flag被设置了，那么把它的值赋给cfg.HTTPHost
			cfg.HTTPHost = ctx.GlobalString(RPCListenAddrFlag.Name)
		}
	}

	if ctx.GlobalIsSet(RPCPortFlag.Name) {
		// 如果rpcport flag的值被设置了，那么把它的值赋给cfg.HTTPPort
		cfg.HTTPPort = ctx.GlobalInt(RPCPortFlag.Name)
	}
	if ctx.GlobalIsSet(RPCCORSDomainFlag.Name) {
		// 如果rpccorsdomain的值被设置了，那么把它的值按逗号分割为切片后赋给cfg.HTTPCors
		cfg.HTTPCors = splitAndTrim(ctx.GlobalString(RPCCORSDomainFlag.Name))
	}
	if ctx.GlobalIsSet(RPCApiFlag.Name) {
		// 如果rpcapi的值被设置了，那么把它的值按逗号分割为切片后赋给cfg.HTTPModules
		cfg.HTTPModules = splitAndTrim(ctx.GlobalString(RPCApiFlag.Name))
	}
	if ctx.GlobalIsSet(RPCVirtualHostsFlag.Name) {
		// 如果rpcvhosts的值被设置了，那么把它的值按逗号分割为切片后赋给cfg.HTTPVirtualHosts
		cfg.HTTPVirtualHosts = splitAndTrim(ctx.GlobalString(RPCVirtualHostsFlag.Name))
	}
}

// setWS creates the WebSocket RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
// 从命令行flag创建WebSocket RPC监听接口
func setWS(ctx *cli.Context, cfg *node.Config) {
	if ctx.GlobalBool(WSEnabledFlag.Name) && cfg.WSHost == "" {
		// 如果ws flag被设置了，并且cfg.WSHost为空，那么把它的值设置为127.0.0.1
		cfg.WSHost = "127.0.0.1"
		if ctx.GlobalIsSet(WSListenAddrFlag.Name) {
			// 如果wsaddr flag被设置了，那么把它的值赋给cfg.WSHost
			cfg.WSHost = ctx.GlobalString(WSListenAddrFlag.Name)
		}
	}

	if ctx.GlobalIsSet(WSPortFlag.Name) {
		// 如果wsport flag被设置了，把它的值赋给cfg.WSPort
		cfg.WSPort = ctx.GlobalInt(WSPortFlag.Name)
	}
	if ctx.GlobalIsSet(WSAllowedOriginsFlag.Name) {
		// 如果wsorigins flag被设置了，把它的值用逗号分割成切片赋给cfg.WSOrigins
		cfg.WSOrigins = splitAndTrim(ctx.GlobalString(WSAllowedOriginsFlag.Name))
	}
	if ctx.GlobalIsSet(WSApiFlag.Name) {
		// 如果wsapi flag被设置了，把它的值用逗号分割成切片赋给cfg.WSModules
		cfg.WSModules = splitAndTrim(ctx.GlobalString(WSApiFlag.Name))
	}
}

// setIPC creates an IPC path configuration from the set command line flags,
// returning an empty string if IPC was explicitly disabled, or the set path.
// 从命令行flag创建一个IPC路径配置，如果IPC被显式禁用，那返回空字符串，否则返回用户设置的值
func setIPC(ctx *cli.Context, cfg *node.Config) {
	// 确保ipcdisable和ipcpath只有其中之一被设置了
	checkExclusive(ctx, IPCDisabledFlag, IPCPathFlag)
	switch {
	case ctx.GlobalBool(IPCDisabledFlag.Name):
		cfg.IPCPath = ""
	case ctx.GlobalIsSet(IPCPathFlag.Name):
		cfg.IPCPath = ctx.GlobalString(IPCPathFlag.Name)
	}
}

// makeDatabaseHandles raises out the number of allowed file handles per process
// for Geth and returns half of the allowance to assign to the database.
// makeDatabaseHandles为Geth提取每个进程允许的文件句柄数，并返回一半分配给数据库。
func makeDatabaseHandles() int {
	// 操作系统允许此进程打开的文件描述符的数量
	limit, err := fdlimit.Current()
	if err != nil {
		Fatalf("Failed to retrieve file descriptor allowance: %v", err)
	}
	if limit < 2048 {
		// 如果操作系统允许的最大文件句柄数小于2048，那么打印错误并且退出程序
		if err := fdlimit.Raise(2048); err != nil {
			Fatalf("Failed to raise file descriptor allowance: %v", err)
		}
	}
	// 限制数据库文件描述符数量，即使有更多可用
	if limit > 2048 { // cap database file descriptors even if more is available
		limit = 2048
	}
	// 留下一半给网络和其他东西
	return limit / 2 // Leave half for networking and other stuff
}

// MakeAddress converts an account specified directly as a hex encoded string or
// a key index in the key store to an internal account representation.
// MakeAddress将直接指定为十六进制编码字符串的帐户或密钥库中的密钥索引转换为内部帐户表示。
func MakeAddress(ks *keystore.KeyStore, account string) (accounts.Account, error) {
	// If the specified account is a valid address, return it
	// 如果指定的帐户是有效地址，将其返回
	if common.IsHexAddress(account) {
		return accounts.Account{Address: common.HexToAddress(account)}, nil
	}
	// Otherwise try to interpret the account as a keystore index
	// 否则尝试将该帐户解释为密钥库索引
	index, err := strconv.Atoi(account)
	if err != nil || index < 0 {
		// %q用于string类型表示使用Go语法安全转义的双引号字符串
		return accounts.Account{}, fmt.Errorf("invalid account address or index %q", account)
	}
	log.Warn("-------------------------------------------------------------------")
	// 在密钥库文件夹中按顺序引用帐户是危险的！
	log.Warn("Referring to accounts by order in the keystore folder is dangerous!")
	// 此功能已弃用，将来会被删除！
	log.Warn("This functionality is deprecated and will be removed in the future!")
	// 请使用明确的地址！ （可以通过`geth account list`搜索）
	log.Warn("Please use explicit addresses! (can search via `geth account list`)")
	log.Warn("-------------------------------------------------------------------")

	// 获取keystore目录中存在的所有账户
	accs := ks.Accounts()
	if len(accs) <= index {
		return accounts.Account{}, fmt.Errorf("index %d higher than number of accounts %d", index, len(accs))
	}
	return accs[index], nil
}

// setEtherbase retrieves the etherbase either from the directly specified
// command line flags or from the keystore if CLI indexed.
// setEtherbase从直接指定的命令行flag或从CLI索引的密钥库中检索etherbase。
func setEtherbase(ctx *cli.Context, ks *keystore.KeyStore, cfg *eth.Config) {
	// Extract the current etherbase, new flag overriding legacy one
	// 提取当前的etherbase，新flag覆盖旧flag
	var etherbase string
	if ctx.GlobalIsSet(MinerLegacyEtherbaseFlag.Name) {
		// etherbase flag
		etherbase = ctx.GlobalString(MinerLegacyEtherbaseFlag.Name)
	}
	if ctx.GlobalIsSet(MinerEtherbaseFlag.Name) {
		// miner.etherbase flag, 优先级高于etherbase flag
		etherbase = ctx.GlobalString(MinerEtherbaseFlag.Name)
	}
	// Convert the etherbase into an address and configure it
	// 将etherbase转换为地址并进行配置
	if etherbase != "" {
		account, err := MakeAddress(ks, etherbase)
		if err != nil {
			Fatalf("Invalid miner etherbase: %v", err)
		}
		// 把矿工账户的地址赋给cfg.Etherbase
		cfg.Etherbase = account.Address
	}
}

// MakePasswordList reads password lines from the file specified by the global --password flag.
func MakePasswordList(ctx *cli.Context) []string {
	path := ctx.GlobalString(PasswordFileFlag.Name)
	if path == "" {
		return nil
	}
	text, err := ioutil.ReadFile(path)
	if err != nil {
		Fatalf("Failed to read password file: %v", err)
	}
	lines := strings.Split(string(text), "\n")
	// Sanitise DOS line endings.
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func SetP2PConfig(ctx *cli.Context, cfg *p2p.Config) {
	// 设置节点的私钥
	setNodeKey(ctx, cfg)
	// 从命令行flag创建一个端口映射器
	setNAT(ctx, cfg)
	// 从命令行flag创建一个TCP监听端口
	setListenAddress(ctx, cfg)
	// 从命令行flag设置引导节点
	setBootstrapNodes(ctx, cfg)
	// 从命令行flag设置v5的引导节点
	setBootstrapNodesV5(ctx, cfg)

	// 如果syncmode flag的值是light，那么说明是轻客户端。默认不是轻客户端，而是FastSync。
	lightClient := ctx.GlobalString(SyncModeFlag.Name) == "light"
	// 如果lightserv flag的值不等于0，那么说明是轻服务器。默认值是0，不是轻服务器。
	lightServer := ctx.GlobalInt(LightServFlag.Name) != 0
	// 轻客户端对等节点的最大数量，默认是100
	lightPeers := ctx.GlobalInt(LightPeersFlag.Name)

	if ctx.GlobalIsSet(MaxPeersFlag.Name) {
		cfg.MaxPeers = ctx.GlobalInt(MaxPeersFlag.Name)
		if lightServer && !ctx.GlobalIsSet(LightPeersFlag.Name) {
			// 如果是轻服务器，设置了maxpeers flag，但是没有设置lightpeers flag，那么就对cfg.MaxPeers加上默认的lightPeers值100
			cfg.MaxPeers += lightPeers
		}
	} else {
		if lightServer {
			// 如果是轻服务器，没有设置maxpeers flag，那么就对cfg.MaxPeers（默认值25）加上lightPeers的值
			cfg.MaxPeers += lightPeers
		}
		if lightClient && ctx.GlobalIsSet(LightPeersFlag.Name) && cfg.MaxPeers < lightPeers {
			// 如果是轻客户端，没有设置maxpeers flag，设置了lightpeers flag，并且MaxPeers小于lightPeers，那么把lightPeers的值赋给
			// MaxPeers，即二者相等
			cfg.MaxPeers = lightPeers
		}
	}
	if !(lightClient || lightServer) {
		// 如果既不是轻客户端，也不是轻服务器，就把lightPeers设置为0
		lightPeers = 0
	}
	// ethPeers代表非轻客户端对等节点
	ethPeers := cfg.MaxPeers - lightPeers
	if lightClient {
		// 如果是轻客户端，就把非轻客户端对等节点的数量设置为0
		ethPeers = 0
	}
	log.Info("Maximum peer count", "ETH", ethPeers, "LES", lightPeers, "total", cfg.MaxPeers)

	if ctx.GlobalIsSet(MaxPendingPeersFlag.Name) {
		cfg.MaxPendingPeers = ctx.GlobalInt(MaxPendingPeersFlag.Name)
	}
	if ctx.GlobalIsSet(NoDiscoverFlag.Name) || lightClient {
		// 如果nodiscover flag被设置了，或者节点是轻客户端，那么就关闭对等节点发现机制
		cfg.NoDiscovery = true
	}

	// if we're running a light client or server, force enable the v5 peer discovery
	// unless it is explicitly disabled with --nodiscover note that explicitly specifying
	// --v5disc overrides --nodiscover, in which case the later only disables v4 discovery
	forceV5Discovery := (lightClient || lightServer) && !ctx.GlobalBool(NoDiscoverFlag.Name)
	if ctx.GlobalIsSet(DiscoveryV5Flag.Name) {
		// 如果v5disc flag被设置，那么DiscoveryV5的值就等于v5disc flag的值
		cfg.DiscoveryV5 = ctx.GlobalBool(DiscoveryV5Flag.Name)
	} else if forceV5Discovery {
		// 如果v5disc flag没有被设置，并且是轻客户端或者轻服务器，并且nodiscover flag没有被设置或者值为false，那么把DiscoveryV5设置为true
		// 也就是开启v5对等节点发现
		cfg.DiscoveryV5 = true
	}

	if netrestrict := ctx.GlobalString(NetrestrictFlag.Name); netrestrict != "" {
		// 如果netrestrict flag的值不为空，就从netrestrict flag的值解析出网段
		list, err := netutil.ParseNetlist(netrestrict)
		if err != nil {
			Fatalf("Option %q: %v", NetrestrictFlag.Name, err)
		}
		cfg.NetRestrict = list
	}

	if ctx.GlobalBool(DeveloperFlag.Name) {
		// --dev mode can't use p2p networking.
		// dev模式被开启的设置如下，不能使用P2P网络
		cfg.MaxPeers = 0
		cfg.ListenAddr = ":0"
		cfg.NoDiscovery = true
		cfg.DiscoveryV5 = false
	}
}

// SetNodeConfig applies node-related command line flags to the config.
// 把和节点相关的命令行flag应用到配置
func SetNodeConfig(ctx *cli.Context, cfg *node.Config) {
	// 把P2P网络相关的flag应用到配置
	SetP2PConfig(ctx, &cfg.P2P)
	// 把IPC相关的flag应用到配置
	setIPC(ctx, cfg)
	// 把HTTP相关的flag应用到配置
	setHTTP(ctx, cfg)
	// 把websocket相关的flag应用到配置
	setWS(ctx, cfg)
	// 把用户标识符相关的flag应用到配置
	setNodeUserIdent(ctx, cfg)

	setDataDir(ctx, cfg)

	if ctx.GlobalIsSet(KeyStoreDirFlag.Name) {
		// 如果keystore flag被设置了，就把它的值赋给cfg.KeyStoreDir
		cfg.KeyStoreDir = ctx.GlobalString(KeyStoreDirFlag.Name)
	}
	if ctx.GlobalIsSet(LightKDFFlag.Name) {
		// 如果lightkdf flag被设置了，就把它的值赋给cfg.UseLightweightKDF
		cfg.UseLightweightKDF = ctx.GlobalBool(LightKDFFlag.Name)
	}
	if ctx.GlobalIsSet(NoUSBFlag.Name) {
		// 如果nousb flag被设置了，就把它的值赋给cfg.NoUSB
		cfg.NoUSB = ctx.GlobalBool(NoUSBFlag.Name)
	}
}

func setDataDir(ctx *cli.Context, cfg *node.Config) {
	switch {
	case ctx.GlobalIsSet(DataDirFlag.Name):
		cfg.DataDir = ctx.GlobalString(DataDirFlag.Name)
	case ctx.GlobalBool(DeveloperFlag.Name):
		cfg.DataDir = "" // unless explicitly requested, use memory databases
	case ctx.GlobalBool(TestnetFlag.Name):
		cfg.DataDir = filepath.Join(node.DefaultDataDir(), "testnet")
	case ctx.GlobalBool(RinkebyFlag.Name):
		cfg.DataDir = filepath.Join(node.DefaultDataDir(), "rinkeby")
	}
}

func setGPO(ctx *cli.Context, cfg *gasprice.Config) {
	if ctx.GlobalIsSet(GpoBlocksFlag.Name) {
		cfg.Blocks = ctx.GlobalInt(GpoBlocksFlag.Name)
	}
	if ctx.GlobalIsSet(GpoPercentileFlag.Name) {
		cfg.Percentile = ctx.GlobalInt(GpoPercentileFlag.Name)
	}
}

func setTxPool(ctx *cli.Context, cfg *core.TxPoolConfig) {
	if ctx.GlobalIsSet(TxPoolLocalsFlag.Name) {
		// txpool.locals flag
		locals := strings.Split(ctx.GlobalString(TxPoolLocalsFlag.Name), ",")
		for _, account := range locals {
			if trimmed := strings.TrimSpace(account); !common.IsHexAddress(trimmed) {
				Fatalf("Invalid account in --txpool.locals: %s", trimmed)
			} else {
				cfg.Locals = append(cfg.Locals, common.HexToAddress(account))
			}
		}
	}
	if ctx.GlobalIsSet(TxPoolNoLocalsFlag.Name) {
		// txpool.nolocals flag
		cfg.NoLocals = ctx.GlobalBool(TxPoolNoLocalsFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolJournalFlag.Name) {
		// txpool.journal flag
		cfg.Journal = ctx.GlobalString(TxPoolJournalFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolRejournalFlag.Name) {
		// txpool.rejournal flag
		cfg.Rejournal = ctx.GlobalDuration(TxPoolRejournalFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolPriceLimitFlag.Name) {
		// txpool.pricelimit flag
		cfg.PriceLimit = ctx.GlobalUint64(TxPoolPriceLimitFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolPriceBumpFlag.Name) {
		// txpool.pricebump flag
		cfg.PriceBump = ctx.GlobalUint64(TxPoolPriceBumpFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolAccountSlotsFlag.Name) {
		// txpool.accountslots flag
		cfg.AccountSlots = ctx.GlobalUint64(TxPoolAccountSlotsFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolGlobalSlotsFlag.Name) {
		// txpool.globalslots flag
		cfg.GlobalSlots = ctx.GlobalUint64(TxPoolGlobalSlotsFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolAccountQueueFlag.Name) {
		// txpool.accountqueue flag
		cfg.AccountQueue = ctx.GlobalUint64(TxPoolAccountQueueFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolGlobalQueueFlag.Name) {
		// txpool.globalqueue flag
		cfg.GlobalQueue = ctx.GlobalUint64(TxPoolGlobalQueueFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolLifetimeFlag.Name) {
		// txpool.lifetime flag
		cfg.Lifetime = ctx.GlobalDuration(TxPoolLifetimeFlag.Name)
	}
}

func setEthash(ctx *cli.Context, cfg *eth.Config) {
	if ctx.GlobalIsSet(EthashCacheDirFlag.Name) {
		// ethash.cachedir flag
		cfg.Ethash.CacheDir = ctx.GlobalString(EthashCacheDirFlag.Name)
	}
	if ctx.GlobalIsSet(EthashDatasetDirFlag.Name) {
		// ethash.dagdir flag
		cfg.Ethash.DatasetDir = ctx.GlobalString(EthashDatasetDirFlag.Name)
	}
	if ctx.GlobalIsSet(EthashCachesInMemoryFlag.Name) {
		// ethash.cachesinmem flag
		cfg.Ethash.CachesInMem = ctx.GlobalInt(EthashCachesInMemoryFlag.Name)
	}
	if ctx.GlobalIsSet(EthashCachesOnDiskFlag.Name) {
		// ethash.cachesondisk flag
		cfg.Ethash.CachesOnDisk = ctx.GlobalInt(EthashCachesOnDiskFlag.Name)
	}
	if ctx.GlobalIsSet(EthashDatasetsInMemoryFlag.Name) {
		// ethash.dagsinmem flag
		cfg.Ethash.DatasetsInMem = ctx.GlobalInt(EthashDatasetsInMemoryFlag.Name)
	}
	if ctx.GlobalIsSet(EthashDatasetsOnDiskFlag.Name) {
		// ethash.dagsondisk flag
		cfg.Ethash.DatasetsOnDisk = ctx.GlobalInt(EthashDatasetsOnDiskFlag.Name)
	}
}

// checkExclusive verifies that only a single instance of the provided flags was
// set by the user. Each flag might optionally be followed by a string type to
// specialize it further.
// 验证参数里面的多个flag其中只有一个被用户设置了，如果不止一个被设置，就退出程序。
func checkExclusive(ctx *cli.Context, args ...interface{}) {
	set := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		// Make sure the next argument is a flag and skip if not set
		flag, ok := args[i].(cli.Flag)
		if !ok {
			panic(fmt.Sprintf("invalid argument, not cli.Flag type: %T", args[i]))
		}
		// Check if next arg extends current and expand its name if so
		name := flag.GetName()

		if i+1 < len(args) {
			switch option := args[i+1].(type) {
			case string:
				// Extended flag check, make sure value set doesn't conflict with passed in option
				if ctx.GlobalString(flag.GetName()) == option {
					name += "=" + option
					set = append(set, "--"+name)
				}
				// shift arguments and continue
				i++
				continue

			case cli.Flag:
			default:
				panic(fmt.Sprintf("invalid argument, not cli.Flag or string extension: %T", args[i+1]))
			}
		}
		// Mark the flag if it's set
		if ctx.GlobalIsSet(flag.GetName()) {
			set = append(set, "--"+name)
		}
	}
	if len(set) > 1 {
		Fatalf("Flags %v can't be used at the same time", strings.Join(set, ", "))
	}
}

// SetShhConfig applies shh-related command line flags to the config.
// SetShhConfig将与shh相关的命令行flag应用于配置。
func SetShhConfig(ctx *cli.Context, stack *node.Node, cfg *whisper.Config) {
	if ctx.GlobalIsSet(WhisperMaxMessageSizeFlag.Name) {
		// shh.maxmessagesize flag
		cfg.MaxMessageSize = uint32(ctx.GlobalUint(WhisperMaxMessageSizeFlag.Name))
	}
	if ctx.GlobalIsSet(WhisperMinPOWFlag.Name) {
		cfg.MinimumAcceptedPOW = ctx.GlobalFloat64(WhisperMinPOWFlag.Name)
	}
	if ctx.GlobalIsSet(WhisperRestrictConnectionBetweenLightClientsFlag.Name) {
		cfg.RestrictConnectionBetweenLightClients = true
	}
}

// SetEthConfig applies eth-related command line flags to the config.
// 把eth相关的命令行flag应用到config
func SetEthConfig(ctx *cli.Context, stack *node.Node, cfg *eth.Config) {
	// Avoid conflicting network flags
	// 避免网络flag冲突
	checkExclusive(ctx, DeveloperFlag, TestnetFlag, RinkebyFlag)
	checkExclusive(ctx, LightServFlag, SyncModeFlag, "light")

	// 获取节点上的第一个KeyStore实例
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	// 从直接指定的命令行flag或从CLI索引的密钥库中检索etherbase
	setEtherbase(ctx, ks, cfg)
	setGPO(ctx, &cfg.GPO)
	// 把交易池相关的flag应用到配置
	setTxPool(ctx, &cfg.TxPool)
	// 把Ethash挖矿的一些flag应用到配置
	setEthash(ctx, cfg)

	if ctx.GlobalIsSet(SyncModeFlag.Name) {
		// syncmode flag
		cfg.SyncMode = *GlobalTextMarshaler(ctx, SyncModeFlag.Name).(*downloader.SyncMode)
	}
	if ctx.GlobalIsSet(LightServFlag.Name) {
		// lightserv flag
		cfg.LightServ = ctx.GlobalInt(LightServFlag.Name)
	}
	if ctx.GlobalIsSet(LightPeersFlag.Name) {
		// lightpeers flag
		cfg.LightPeers = ctx.GlobalInt(LightPeersFlag.Name)
	}
	if ctx.GlobalIsSet(NetworkIdFlag.Name) {
		cfg.NetworkId = ctx.GlobalUint64(NetworkIdFlag.Name)
	}

	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheDatabaseFlag.Name) {
		// 如果cache flag或cache.database flag其中之一被设置
		cfg.DatabaseCache = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheDatabaseFlag.Name) / 100
	}
	// 为Geth提取每个进程允许的文件句柄数，并返回一半分配给数据库。
	cfg.DatabaseHandles = makeDatabaseHandles()

	if gcmode := ctx.GlobalString(GCModeFlag.Name); gcmode != "full" && gcmode != "archive" {
		// gcmode flag
		Fatalf("--%s must be either 'full' or 'archive'", GCModeFlag.Name)
	}
	cfg.NoPruning = ctx.GlobalString(GCModeFlag.Name) == "archive"

	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheTrieFlag.Name) {
		cfg.TrieCleanCache = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheTrieFlag.Name) / 100
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheGCFlag.Name) {
		cfg.TrieDirtyCache = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheGCFlag.Name) / 100
	}
	if ctx.GlobalIsSet(MinerNotifyFlag.Name) {
		// miner.notify flag
		cfg.MinerNotify = strings.Split(ctx.GlobalString(MinerNotifyFlag.Name), ",")
	}
	if ctx.GlobalIsSet(DocRootFlag.Name) {
		// docroot flag
		cfg.DocRoot = ctx.GlobalString(DocRootFlag.Name)
	}
	if ctx.GlobalIsSet(MinerLegacyExtraDataFlag.Name) {
		// extradata flag
		cfg.MinerExtraData = []byte(ctx.GlobalString(MinerLegacyExtraDataFlag.Name))
	}
	if ctx.GlobalIsSet(MinerExtraDataFlag.Name) {
		// miner.extradata flag
		cfg.MinerExtraData = []byte(ctx.GlobalString(MinerExtraDataFlag.Name))
	}
	if ctx.GlobalIsSet(MinerLegacyGasTargetFlag.Name) {
		// targetgaslimit flag
		cfg.MinerGasFloor = ctx.GlobalUint64(MinerLegacyGasTargetFlag.Name)
	}
	if ctx.GlobalIsSet(MinerGasTargetFlag.Name) {
		// miner.gastarget flag
		cfg.MinerGasFloor = ctx.GlobalUint64(MinerGasTargetFlag.Name)
	}
	if ctx.GlobalIsSet(MinerGasLimitFlag.Name) {
		// miner.gaslimit flag
		cfg.MinerGasCeil = ctx.GlobalUint64(MinerGasLimitFlag.Name)
	}
	if ctx.GlobalIsSet(MinerLegacyGasPriceFlag.Name) {
		// gasprice flag
		cfg.MinerGasPrice = GlobalBig(ctx, MinerLegacyGasPriceFlag.Name)
	}
	if ctx.GlobalIsSet(MinerGasPriceFlag.Name) {
		// miner.gasprice flag
		cfg.MinerGasPrice = GlobalBig(ctx, MinerGasPriceFlag.Name)
	}
	if ctx.GlobalIsSet(MinerRecommitIntervalFlag.Name) {
		// miner.recommit flag
		cfg.MinerRecommit = ctx.Duration(MinerRecommitIntervalFlag.Name)
	}
	if ctx.GlobalIsSet(MinerNoVerfiyFlag.Name) {
		// miner.noverify flag
		cfg.MinerNoverify = ctx.Bool(MinerNoVerfiyFlag.Name)
	}
	if ctx.GlobalIsSet(VMEnableDebugFlag.Name) {
		// vmdebug flag
		// TODO(fjl): force-enable this in --dev mode
		cfg.EnablePreimageRecording = ctx.GlobalBool(VMEnableDebugFlag.Name)
	}

	if ctx.GlobalIsSet(EWASMInterpreterFlag.Name) {
		cfg.EWASMInterpreter = ctx.GlobalString(EWASMInterpreterFlag.Name)
	}

	if ctx.GlobalIsSet(EVMInterpreterFlag.Name) {
		cfg.EVMInterpreter = ctx.GlobalString(EVMInterpreterFlag.Name)
	}

	// Override any default configs for hard coded networks.
	// 覆盖硬编码网络的任何默认配置。
	switch {
	case ctx.GlobalBool(TestnetFlag.Name):
		// 如果testnet flag设置为true，并且networkid flag没有设置，那么把NetworkId设置为3（代表ropsten）；
		// 如果networkid flag已经设置了，那testnet flag就不生效。说明networkid flag优先级更高
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			cfg.NetworkId = 3
		}
		cfg.Genesis = core.DefaultTestnetGenesisBlock()
	case ctx.GlobalBool(RinkebyFlag.Name):
		// rinkeby flag
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			cfg.NetworkId = 4
		}
		cfg.Genesis = core.DefaultRinkebyGenesisBlock()
	case ctx.GlobalBool(DeveloperFlag.Name):
		// dev flag
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			cfg.NetworkId = 1337
		}
		// Create new developer account or reuse existing one
		// 创建新的开发者帐户或重用现有帐户
		var (
			developer accounts.Account
			err       error
		)
		if accs := ks.Accounts(); len(accs) > 0 {
			// 把keystore缓存的第一个账户赋给开发者账户
			developer = ks.Accounts()[0]
		} else {
			// 新建一个开发者账户，密码是空字符串
			developer, err = ks.NewAccount("")
			if err != nil {
				Fatalf("Failed to create developer account: %v", err)
			}
		}
		// 无限期解锁开发者账户
		if err := ks.Unlock(developer, ""); err != nil {
			Fatalf("Failed to unlock developer account: %v", err)
		}
		log.Info("Using developer account", "address", developer.Address)

		cfg.Genesis = core.DeveloperGenesisBlock(uint64(ctx.GlobalInt(DeveloperPeriodFlag.Name)), developer.Address)
		if !ctx.GlobalIsSet(MinerGasPriceFlag.Name) && !ctx.GlobalIsSet(MinerLegacyGasPriceFlag.Name) {
			cfg.MinerGasPrice = big.NewInt(1)
		}
	}
	// TODO(fjl): move trie cache generations into config
	if gen := ctx.GlobalInt(TrieCacheGenFlag.Name); gen > 0 {
		state.MaxTrieCacheGen = uint16(gen)
	}
}

// SetDashboardConfig applies dashboard related command line flags to the config.
// SetDashboardConfig将与仪表板相关的命令行flag应用于配置。
func SetDashboardConfig(ctx *cli.Context, cfg *dashboard.Config) {
	cfg.Host = ctx.GlobalString(DashboardAddrFlag.Name)
	cfg.Port = ctx.GlobalInt(DashboardPortFlag.Name)
	cfg.Refresh = ctx.GlobalDuration(DashboardRefreshFlag.Name)
}

// RegisterEthService adds an Ethereum client to the stack.
// RegisterEthService将以太坊客户端添加到堆栈。
func RegisterEthService(stack *node.Node, cfg *eth.Config) {
	var err error
	if cfg.SyncMode == downloader.LightSync {
		err = stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
			// 创建一个以太坊轻客户端
			return les.New(ctx, cfg)
		})
	} else {
		err = stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
			// New创建一个新的以太坊对象（包括一般的以太坊对象的初始化）
			fullNode, err := eth.New(ctx, cfg)
			if fullNode != nil && cfg.LightServ > 0 {
				// TODO Corey 为什么不用判断les.NewLesServer返回的error？
				ls, _ := les.NewLesServer(fullNode, cfg)
				fullNode.AddLesServer(ls)
			}
			return fullNode, err
		})
	}
	if err != nil {
		// 无法注册以太坊服务
		Fatalf("Failed to register the Ethereum service: %v", err)
	}
}

// RegisterDashboardService adds a dashboard to the stack.
// RegisterDashboardService将一个仪表板添加到堆栈。
func RegisterDashboardService(stack *node.Node, cfg *dashboard.Config, commit string) {
	stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		return dashboard.New(cfg, commit, ctx.ResolvePath("logs")), nil
	})
}

// RegisterShhService configures Whisper and adds it to the given node.
// RegisterShhService配置Whisper并将其添加到给定节点。
func RegisterShhService(stack *node.Node, cfg *whisper.Config) {
	if err := stack.Register(func(n *node.ServiceContext) (node.Service, error) {
		return whisper.New(cfg), nil
	}); err != nil {
		Fatalf("Failed to register the Whisper service: %v", err)
	}
}

// RegisterEthStatsService configures the Ethereum Stats daemon and adds it to
// the given node.
// RegisterEthStatsService配置以太坊统计守护程序并将其添加到给定节点。
func RegisterEthStatsService(stack *node.Node, url string) {
	if err := stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		// Retrieve both eth and les services
		var ethServ *eth.Ethereum
		ctx.Service(&ethServ)

		var lesServ *les.LightEthereum
		ctx.Service(&lesServ)

		return ethstats.New(url, ethServ, lesServ)
	}); err != nil {
		Fatalf("Failed to register the Ethereum Stats service: %v", err)
	}
}

func SetupMetrics(ctx *cli.Context) {
	if metrics.Enabled {
		log.Info("Enabling metrics collection")
		var (
			enableExport = ctx.GlobalBool(MetricsEnableInfluxDBFlag.Name)
			endpoint     = ctx.GlobalString(MetricsInfluxDBEndpointFlag.Name)
			database     = ctx.GlobalString(MetricsInfluxDBDatabaseFlag.Name)
			username     = ctx.GlobalString(MetricsInfluxDBUsernameFlag.Name)
			password     = ctx.GlobalString(MetricsInfluxDBPasswordFlag.Name)
			hosttag      = ctx.GlobalString(MetricsInfluxDBHostTagFlag.Name)
		)

		if enableExport {
			log.Info("Enabling metrics export to InfluxDB")
			go influxdb.InfluxDBWithTags(metrics.DefaultRegistry, 10*time.Second, endpoint, database, username, password, "geth.", map[string]string{
				"host": hosttag,
			})
		}
	}
}

// MakeChainDatabase open an LevelDB using the flags passed to the client and will hard crash if it fails.
func MakeChainDatabase(ctx *cli.Context, stack *node.Node) ethdb.Database {
	var (
		cache   = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheDatabaseFlag.Name) / 100
		handles = makeDatabaseHandles()
	)
	name := "chaindata"
	if ctx.GlobalString(SyncModeFlag.Name) == "light" {
		name = "lightchaindata"
	}
	chainDb, err := stack.OpenDatabase(name, cache, handles)
	if err != nil {
		Fatalf("Could not open database: %v", err)
	}
	return chainDb
}

func MakeGenesis(ctx *cli.Context) *core.Genesis {
	var genesis *core.Genesis
	switch {
	case ctx.GlobalBool(TestnetFlag.Name):
		genesis = core.DefaultTestnetGenesisBlock()
	case ctx.GlobalBool(RinkebyFlag.Name):
		genesis = core.DefaultRinkebyGenesisBlock()
	case ctx.GlobalBool(DeveloperFlag.Name):
		Fatalf("Developer chains are ephemeral")
	}
	return genesis
}

// MakeChain creates a chain manager from set command line flags.
func MakeChain(ctx *cli.Context, stack *node.Node) (chain *core.BlockChain, chainDb ethdb.Database) {
	var err error
	chainDb = MakeChainDatabase(ctx, stack)

	config, _, err := core.SetupGenesisBlock(chainDb, MakeGenesis(ctx))
	if err != nil {
		Fatalf("%v", err)
	}
	var engine consensus.Engine
	if config.Clique != nil {
		engine = clique.New(config.Clique, chainDb)
	} else {
		engine = ethash.NewFaker()
		if !ctx.GlobalBool(FakePoWFlag.Name) {
			engine = ethash.New(ethash.Config{
				CacheDir:       stack.ResolvePath(eth.DefaultConfig.Ethash.CacheDir),
				CachesInMem:    eth.DefaultConfig.Ethash.CachesInMem,
				CachesOnDisk:   eth.DefaultConfig.Ethash.CachesOnDisk,
				DatasetDir:     stack.ResolvePath(eth.DefaultConfig.Ethash.DatasetDir),
				DatasetsInMem:  eth.DefaultConfig.Ethash.DatasetsInMem,
				DatasetsOnDisk: eth.DefaultConfig.Ethash.DatasetsOnDisk,
			}, nil, false)
		}
	}
	if gcmode := ctx.GlobalString(GCModeFlag.Name); gcmode != "full" && gcmode != "archive" {
		Fatalf("--%s must be either 'full' or 'archive'", GCModeFlag.Name)
	}
	cache := &core.CacheConfig{
		Disabled:       ctx.GlobalString(GCModeFlag.Name) == "archive",
		TrieCleanLimit: eth.DefaultConfig.TrieCleanCache,
		TrieDirtyLimit: eth.DefaultConfig.TrieDirtyCache,
		TrieTimeLimit:  eth.DefaultConfig.TrieTimeout,
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheTrieFlag.Name) {
		cache.TrieCleanLimit = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheTrieFlag.Name) / 100
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheGCFlag.Name) {
		cache.TrieDirtyLimit = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheGCFlag.Name) / 100
	}
	vmcfg := vm.Config{EnablePreimageRecording: ctx.GlobalBool(VMEnableDebugFlag.Name)}
	chain, err = core.NewBlockChain(chainDb, cache, config, engine, vmcfg, nil)
	if err != nil {
		Fatalf("Can't create BlockChain: %v", err)
	}
	return chain, chainDb
}

// MakeConsolePreloads retrieves the absolute paths for the console JavaScript
// scripts to preload before starting.
func MakeConsolePreloads(ctx *cli.Context) []string {
	// Skip preloading if there's nothing to preload
	if ctx.GlobalString(PreloadJSFlag.Name) == "" {
		return nil
	}
	// Otherwise resolve absolute paths and return them
	preloads := []string{}

	assets := ctx.GlobalString(JSpathFlag.Name)
	for _, file := range strings.Split(ctx.GlobalString(PreloadJSFlag.Name), ",") {
		preloads = append(preloads, common.AbsolutePath(assets, strings.TrimSpace(file)))
	}
	return preloads
}

// MigrateFlags sets the global flag from a local flag when it's set.
// This is a temporary function used for migrating old command/flags to the
// new format.
//
// e.g. geth account new --keystore /tmp/mykeystore --lightkdf
//
// is equivalent after calling this method with:
//
// geth --keystore /tmp/mykeystore --lightkdf account new
//
// This allows the use of the existing configuration functionality.
// When all flags are migrated this function can be removed and the existing
// configuration functionality must be changed that is uses local flags
// MigrateFlags在设置时从本地flag设置全局flag。
// 这是一个临时函数，用于将旧命令/flag迁移到新格式。
// e.g. geth account new --keystore /tmp/mykeystore --lightkdf
// 在调用此方法后，它是等效的：
// geth --keystore /tmp/mykeystore --lightkdf account new
// 这允许使用现有的配置功能。
// 迁移所有flag后，可以删除此功能，并且必须更改使用本地flag的现有配置功能
func MigrateFlags(action func(ctx *cli.Context) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, name := range ctx.FlagNames() {
			if ctx.IsSet(name) {
				ctx.GlobalSet(name, ctx.String(name))
			}
		}
		return action(ctx)
	}
}
