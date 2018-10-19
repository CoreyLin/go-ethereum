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

package node

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/accounts/usbwallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	datadirPrivateKey      = "nodekey"            // Path within the datadir to the node's private key
	datadirDefaultKeyStore = "keystore"           // Path within the datadir to the keystore
	datadirStaticNodes     = "static-nodes.json"  // Path within the datadir to the static node list
	datadirTrustedNodes    = "trusted-nodes.json" // Path within the datadir to the trusted node list
	datadirNodeDatabase    = "nodes"              // Path within the datadir to store the node infos
)

// Config represents a small collection of configuration values to fine tune the
// P2P network layer of a protocol stack. These values can be further extended by
// all registered services.
// Config代表一个小的配置值的集合，用于微调协议栈的点对点网络层。这些配置值可以被所有注册的服务进一步扩展
type Config struct {
	// Name sets the instance name of the node. It must not contain the / character and is
	// used in the devp2p node identifier. The instance name of geth is "geth". If no
	// value is specified, the basename of the current executable is used.
	// 代表节点的实例名。不能包含/以及\，被用在devp2p节点标识符。geth的实例名是geth。如果值没有被指定，当前可执行文件的basename就是实例名
	Name string `toml:"-"`

	// UserIdent, if set, is used as an additional component in the devp2p node identifier.
	// 如果设置了，就在devp2p节点标识符里被作为附加的模块
	UserIdent string `toml:",omitempty"`

	// Version should be set to the version number of the program. It is used
	// in the devp2p node identifier.
	// 应该被设置为程序的版本号。用在devp2p节点标识符里面。
	Version string `toml:"-"`

	// DataDir is the file system folder the node should use for any data storage
	// requirements. The configured data directory will not be directly shared with
	// registered services, instead those can use utility methods to create/access
	// databases or flat files. This enables ephemeral nodes which can fully reside
	// in memory.
	// 用于该节点存储任何数据的文件系统目录。配置的数据目录不会被直接和注册的服务共享，相反，注册的服务可以使用utility方法来创建
	// 或者访问数据库或者平面文件。这使得完全存在于内存中的短暂的节点成为可能。
	DataDir string

	// Configuration of peer-to-peer networking.
	// 点对点网络的配置
	P2P p2p.Config

	// KeyStoreDir is the file system folder that contains private keys. The directory can
	// be specified as a relative path, in which case it is resolved relative to the
	// current directory.
	//
	// If KeyStoreDir is empty, the default location is the "keystore" subdirectory of
	// DataDir. If DataDir is unspecified and KeyStoreDir is empty, an ephemeral directory
	// is created by New and destroyed when the node is stopped.
	// 用于保存私钥的文件系统目录。可以被指定为相对于当前路径的路径。
	// 如果为空，那么默认的路径是DataDir下的keystore子文件夹。如果DataDir没有指定，KeyStoreDir也是空，那么一个短暂的文件夹
	// 被创建，并且在节点停止的时候被删除。
	KeyStoreDir string `toml:",omitempty"`

	// UseLightweightKDF lowers the memory and CPU requirements of the key store
	// scrypt KDF at the expense of security.
	// 以牺牲安全性为代价，降低key store scrypt KDF的内存和CPU需求
	UseLightweightKDF bool `toml:",omitempty"`

	// NoUSB disables hardware wallet monitoring and connectivity.
	// 禁止硬件钱包监控和连接
	NoUSB bool `toml:",omitempty"`

	// IPCPath is the requested location to place the IPC endpoint. If the path is
	// a simple file name, it is placed inside the data directory (or on the root
	// pipe path on Windows), whereas if it's a resolvable path name (absolute or
	// relative), then that specific path is enforced. An empty path disables IPC.
	// 用于定位IPC endpoint的被请求地址。
	IPCPath string `toml:",omitempty"`

	// HTTPHost is the host interface on which to start the HTTP RPC server. If this
	// field is empty, no HTTP API endpoint will be started.
	// 用于启动HTTP RPC服务器的主机接口。如果为空，那么HTTP API endpoint不会启动
	HTTPHost string `toml:",omitempty"`

	// HTTPPort is the TCP port number on which to start the HTTP RPC server. The
	// default zero value is/ valid and will pick a port number randomly (useful
	// for ephemeral nodes).
	// 用于启动HTTP RPC服务器的TCP端口号。默认零值是/,会随机挑选一个端口号（对短暂的节点很有用）
	HTTPPort int `toml:",omitempty"`

	// HTTPCors is the Cross-Origin Resource Sharing header to send to requesting
	// clients. Please be aware that CORS is a browser enforced security, it's fully
	// useless for custom HTTP clients.
	HTTPCors []string `toml:",omitempty"`

	// HTTPVirtualHosts is the list of virtual hostnames which are allowed on incoming requests.
	// This is by default {'localhost'}. Using this prevents attacks like
	// DNS rebinding, which bypasses SOP by simply masquerading as being within the same
	// origin. These attacks do not utilize CORS, since they are not cross-domain.
	// By explicitly checking the Host-header, the server will not allow requests
	// made against the server with a malicious host domain.
	// Requests using ip address directly are not affected
	HTTPVirtualHosts []string `toml:",omitempty"`

	// HTTPModules is a list of API modules to expose via the HTTP RPC interface.
	// If the module list is empty, all RPC API endpoints designated public will be
	// exposed.
	// 通过HTTP RPC接口暴露的API模块的列表。如果为空，所有public的RPC API endpoints都会被暴露。
	HTTPModules []string `toml:",omitempty"`

	// HTTPTimeouts allows for customization of the timeout values used by the HTTP RPC
	// interface.
	// 允许自定义HTTP RPC接口的timeout时间
	HTTPTimeouts rpc.HTTPTimeouts

	// WSHost is the host interface on which to start the websocket RPC server. If
	// this field is empty, no websocket API endpoint will be started.
	// 用于启动websocket RPC服务器的主机接口。如果为空，那么不会启动websocket API endpoint。
	WSHost string `toml:",omitempty"`

	// WSPort is the TCP port number on which to start the websocket RPC server. The
	// default zero value is/ valid and will pick a port number randomly (useful for
	// ephemeral nodes).
	// 用于启动websocket RPC服务器的TCP端口号。默认零值是/,会随机挑选一个端口号（对短暂的节点很有用）
	WSPort int `toml:",omitempty"`

	// WSOrigins is the list of domain to accept websocket requests from. Please be
	// aware that the server can only act upon the HTTP request the client sends and
	// cannot verify the validity of the request header.
	// 指定可以从哪些domain接收websocket请求。仅对这些domain接收到的请求有动作，不能验证请求头的合法性。
	WSOrigins []string `toml:",omitempty"`

	// WSModules is a list of API modules to expose via the websocket RPC interface.
	// If the module list is empty, all RPC API endpoints designated public will be
	// exposed.
	// 通过websocket RPC接口暴露的API模块的列表。如果为空，所有public的RPC API endpoints都会被暴露。
	WSModules []string `toml:",omitempty"`

	// WSExposeAll exposes all API modules via the WebSocket RPC interface rather
	// than just the public ones.
	//
	// *WARNING* Only set this if the node is running in a trusted network, exposing
	// private APIs to untrusted users is a major security risk.
	// 通过WebSocket RPC接口暴露所有的API模块，而不仅仅是暴露public的API。
	// 仅仅在可信任的网络上运行的时候才可以设置为true，把私有API暴露给不可信任的用户是很严重的安全风险。
	WSExposeAll bool `toml:",omitempty"`

	// Logger is a custom logger to use with the p2p.Server.
	// 用于点对点服务器的定制logger
	Logger log.Logger `toml:",omitempty"`
}

// IPCEndpoint resolves an IPC endpoint based on a configured value, taking into
// account the set data folders as well as the designated platform we're currently
// running on.
func (c *Config) IPCEndpoint() string {
	// Short circuit if IPC has not been enabled
	if c.IPCPath == "" {
		return ""
	}
	// On windows we can only use plain top-level pipes
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(c.IPCPath, `\\.\pipe\`) {
			return c.IPCPath
		}
		return `\\.\pipe\` + c.IPCPath
	}
	// Resolve names into the data directory full paths otherwise
	if filepath.Base(c.IPCPath) == c.IPCPath {
		if c.DataDir == "" {
			return filepath.Join(os.TempDir(), c.IPCPath)
		}
		return filepath.Join(c.DataDir, c.IPCPath)
	}
	return c.IPCPath
}

// NodeDB returns the path to the discovery node database.
func (c *Config) NodeDB() string {
	if c.DataDir == "" {
		return "" // ephemeral
	}
	return c.ResolvePath(datadirNodeDatabase)
}

// DefaultIPCEndpoint returns the IPC path used by default.
func DefaultIPCEndpoint(clientIdentifier string) string {
	if clientIdentifier == "" {
		clientIdentifier = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if clientIdentifier == "" {
			panic("empty executable name")
		}
	}
	config := &Config{DataDir: DefaultDataDir(), IPCPath: clientIdentifier + ".ipc"}
	return config.IPCEndpoint()
}

// HTTPEndpoint resolves an HTTP endpoint based on the configured host interface
// and port parameters.
func (c *Config) HTTPEndpoint() string {
	if c.HTTPHost == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}

// DefaultHTTPEndpoint returns the HTTP endpoint used by default.
func DefaultHTTPEndpoint() string {
	config := &Config{HTTPHost: DefaultHTTPHost, HTTPPort: DefaultHTTPPort}
	return config.HTTPEndpoint()
}

// WSEndpoint resolves a websocket endpoint based on the configured host interface
// and port parameters.
func (c *Config) WSEndpoint() string {
	if c.WSHost == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.WSHost, c.WSPort)
}

// DefaultWSEndpoint returns the websocket endpoint used by default.
func DefaultWSEndpoint() string {
	config := &Config{WSHost: DefaultWSHost, WSPort: DefaultWSPort}
	return config.WSEndpoint()
}

// NodeName returns the devp2p node identifier.
func (c *Config) NodeName() string {
	name := c.name()
	// Backwards compatibility: previous versions used title-cased "Geth", keep that.
	if name == "geth" || name == "geth-testnet" {
		name = "Geth"
	}
	if c.UserIdent != "" {
		name += "/" + c.UserIdent
	}
	if c.Version != "" {
		name += "/v" + c.Version
	}
	name += "/" + runtime.GOOS + "-" + runtime.GOARCH
	name += "/" + runtime.Version()
	return name
}

func (c *Config) name() string {
	if c.Name == "" {
		progname := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if progname == "" {
			panic("empty executable name, set Config.Name")
		}
		return progname
	}
	return c.Name
}

// These resources are resolved differently for "geth" instances.
var isOldGethResource = map[string]bool{
	"chaindata":          true,
	"nodes":              true,
	"nodekey":            true,
	"static-nodes.json":  true,
	"trusted-nodes.json": true,
}

// ResolvePath resolves path in the instance directory.
// ResolvePath解析实例目录中的路径。
func (c *Config) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if c.DataDir == "" {
		return ""
	}
	// Backwards-compatibility: ensure that data directory files created
	// by geth 1.4 are used if they exist.
	if c.name() == "geth" && isOldGethResource[path] {
		oldpath := ""
		if c.Name == "geth" {
			oldpath = filepath.Join(c.DataDir, path)
		}
		if oldpath != "" && common.FileExist(oldpath) {
			// TODO: print warning
			return oldpath
		}
	}
	return filepath.Join(c.instanceDir(), path)
}

func (c *Config) instanceDir() string {
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.DataDir, c.name())
}

// NodeKey retrieves the currently configured private key of the node, checking
// first any manually set key, falling back to the one found in the configured
// data folder. If no key can be found, a new one is generated.
func (c *Config) NodeKey() *ecdsa.PrivateKey {
	// Use any specifically configured key.
	if c.P2P.PrivateKey != nil {
		return c.P2P.PrivateKey
	}
	// Generate ephemeral key if no datadir is being used.
	if c.DataDir == "" {
		key, err := crypto.GenerateKey()
		if err != nil {
			log.Crit(fmt.Sprintf("Failed to generate ephemeral node key: %v", err))
		}
		return key
	}

	keyfile := c.ResolvePath(datadirPrivateKey)
	if key, err := crypto.LoadECDSA(keyfile); err == nil {
		return key
	}
	// No persistent key found, generate and store a new one.
	key, err := crypto.GenerateKey()
	if err != nil {
		log.Crit(fmt.Sprintf("Failed to generate node key: %v", err))
	}
	instanceDir := filepath.Join(c.DataDir, c.name())
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
		return key
	}
	keyfile = filepath.Join(instanceDir, datadirPrivateKey)
	if err := crypto.SaveECDSA(keyfile, key); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
	}
	return key
}

// StaticNodes returns a list of node enode URLs configured as static nodes.
func (c *Config) StaticNodes() []*discover.Node {
	return c.parsePersistentNodes(c.ResolvePath(datadirStaticNodes))
}

// TrustedNodes returns a list of node enode URLs configured as trusted nodes.
func (c *Config) TrustedNodes() []*discover.Node {
	return c.parsePersistentNodes(c.ResolvePath(datadirTrustedNodes))
}

// parsePersistentNodes parses a list of discovery node URLs loaded from a .json
// file from within the data directory.
func (c *Config) parsePersistentNodes(path string) []*discover.Node {
	// Short circuit if no node config is present
	if c.DataDir == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	// Load the nodes from the config file.
	var nodelist []string
	if err := common.LoadJSON(path, &nodelist); err != nil {
		log.Error(fmt.Sprintf("Can't load node file %s: %v", path, err))
		return nil
	}
	// Interpret the list as a discovery node array
	var nodes []*discover.Node
	for _, url := range nodelist {
		if url == "" {
			continue
		}
		node, err := discover.ParseNode(url)
		if err != nil {
			log.Error(fmt.Sprintf("Node URL %s: %v\n", url, err))
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// AccountConfig determines the settings for scrypt and keydirectory
// 确定Scrypt加密算法的设置，以及保存私钥的文件夹的绝对路径
func (c *Config) AccountConfig() (int, int, string, error) {
	scryptN := keystore.StandardScryptN
	scryptP := keystore.StandardScryptP
	if c.UseLightweightKDF {
		scryptN = keystore.LightScryptN
		scryptP = keystore.LightScryptP
	}

	var (
		keydir string
		err    error
	)
	switch {
	case filepath.IsAbs(c.KeyStoreDir):
		// 如果KeyStoreDir是绝对路径
		keydir = c.KeyStoreDir
	case c.DataDir != "":
		if c.KeyStoreDir == "" {
			// 如果DataDir不为空，并且KeyStoreDir为空，那么keydir路径就为DataDir加上“keystore”
			keydir = filepath.Join(c.DataDir, datadirDefaultKeyStore)
		} else {
			// 如果DataDir不为空，并且KeyStoreDir不为空，那么keydir路径就为KeyStoreDir所代表的绝对路径
			keydir, err = filepath.Abs(c.KeyStoreDir)
		}
	case c.KeyStoreDir != "":
		// 如果DataDir为空，并且KeyStoreDir不为空，那么keydir路径就为KeyStoreDir所代表的绝对路径
		keydir, err = filepath.Abs(c.KeyStoreDir)
	}
	// 还有一种情况：如果DataDir为空，并且KeyStoreDir也为空，那么keydir为空字符串, err为nil
	return scryptN, scryptP, keydir, err
}

func makeAccountManager(conf *Config) (*accounts.Manager, string, error) {
	// 获取Scrypt加密算法的设置，以及保存私钥的文件夹的绝对路径
	scryptN, scryptP, keydir, err := conf.AccountConfig()
	var ephemeral string
	if keydir == "" {
		// There is no datadir.
		// keydir为空说明datadir没有设置，那么就在操作系统默认的临时目录下创建一个前缀为"go-ethereum-keystore"的目录
		keydir, err = ioutil.TempDir("", "go-ethereum-keystore")
		// 这种方式创建的私钥目录是一个暂时的目录
		ephemeral = keydir
	}

	if err != nil {
		return nil, "", err
	}
	// 如果keydir目录还没有被创建，就新建这个目录，并且权限是0700
	if err := os.MkdirAll(keydir, 0700); err != nil {
		return nil, "", err
	}
	// Assemble the account manager and supported backends
	// 创建一个KeyStore实例
	backends := []accounts.Backend{
		// 新建并初始化一个KeyStore实例
		keystore.NewKeyStore(keydir, scryptN, scryptP),
	}
	if !conf.NoUSB {
		// Start a USB hub for Ledger hardware wallets
		// 为Ledger硬件钱包启动一个USB hub，并且添加到backends切片中
		if ledgerhub, err := usbwallet.NewLedgerHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Ledger hub, disabling: %v", err))
		} else {
			backends = append(backends, ledgerhub)
		}
		// Start a USB hub for Trezor hardware wallets
		// 为Trezor硬件钱包启动一个USB hub，并且添加到backends切片中
		if trezorhub, err := usbwallet.NewTrezorHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Trezor hub, disabling: %v", err))
		} else {
			backends = append(backends, trezorhub)
		}
	}
	return accounts.NewManager(backends...), ephemeral, nil
}
