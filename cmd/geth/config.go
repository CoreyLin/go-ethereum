// Copyright 2017 The go-ethereum Authors
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

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"unicode"

	cli "gopkg.in/urfave/cli.v1"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/dashboard"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv6"
	"github.com/naoina/toml"
)

var (
	dumpConfigCommand = cli.Command{
		Action:      utils.MigrateFlags(dumpConfig),
		Name:        "dumpconfig",
		Usage:       "Show configuration values",
		ArgsUsage:   "",
		Flags:       append(append(nodeFlags, rpcFlags...), whisperFlags...),
		Category:    "MISCELLANEOUS COMMANDS",
		Description: `The dumpconfig command shows configuration values.`,
	}

	configFileFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}
)

// These settings ensure that TOML keys use the same names as Go struct fields.
// 这些设置确保TOML文件里的键和Go struct里的属性用的是相同的名称
// Config包含编码和解码的选项
var tomlSettings = toml.Config{
	// NormFieldName用于匹配TOML键和struct属性，根据TOML键找struct属性。这个函数应该返回一个让这两者匹配的字符串。你必须设置NormFieldName才能使用解码器。
	// 此处参数里的key为TOML键，返回的key为struct属性，二者严格相等
	NormFieldName: func(rt reflect.Type, key string) string {
		return key
	},
	// FieldToKey决定在编码时一个struct属性对应的TOML键。你必须设置FieldToKey才能使用编码器。
	// 此处参数里的key为struct属性，返回的key为TOML键，二者严格相等
	FieldToKey: func(rt reflect.Type, field string) string {
		return field
	},
	// MissingField如果设置为非nil，那么当解码器在遇到无法根据TOML键找到对应的struct属性的时候被调用。
	// 默认行为是返回一个error。
	MissingField: func(rt reflect.Type, field string) error {
		link := ""
		// 如果Go类型名称的第一个字符是大写的，也就是代表公开的，并且Go类型的包路径不为main，那么就加上相关godoc信息的打印
		if unicode.IsUpper(rune(rt.Name()[0])) && rt.PkgPath() != "main" {
			link = fmt.Sprintf(", see https://godoc.org/%s#%s for available fields", rt.PkgPath(), rt.Name())
		}
		return fmt.Errorf("field '%s' is not defined in %s%s", field, rt.String(), link)
	},
}

type ethstatsConfig struct {
	URL string `toml:",omitempty"`
}

// gethConfig代表一个geth客户端的总体，所有的配置，包含多个方面的子配置，如Eth,Shh...
type gethConfig struct {
	Eth       eth.Config
	Shh       whisper.Config
	Node      node.Config
	Ethstats  ethstatsConfig
	Dashboard dashboard.Config
}

func loadConfig(file string, cfg *gethConfig) error {
	// 以只读模式打开配置文件。如果成功，返回的*File可以被用于读取文件内容；相关联的文件描述符具有O_RDONLY模式。
	// 如果有error发生，类型是*PathError
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	// 函数执行完后关闭配置文件
	defer f.Close()

	// bufio.NewReader(f)从返回一个读取TOML配置文件的Reader，buffer size是默认的。
	// tomlSettings.NewDecoder返回一个解码器，这个解码器从TOML配置文件的Reader读取数据。先读取所有数据，然后再解析。
	// Decode函数解析输入的TOML数据，并存储到对应的Go struct里面去。
	// 总的来说，这行代码就是解析TOML配置文件的内容，然后存储到一个gethConfig实例中去。由于这里传入的gethConfig实例之前已经
	// 设置了一些属性的值了，个人认为如果TOML配置文件里定义的键和gethConfig实例有重复，那么TOML配置文件的应该会覆盖掉gethConfig实例里的值。
	err = tomlSettings.NewDecoder(bufio.NewReader(f)).Decode(cfg)
	// Add file name to errors that have a line number.
	// 此处语法参考类型断言：https://golang.org/ref/spec#Type_assertions
	if _, ok := err.(*toml.LineError); ok {
		// 如果err是LineError类型的，那么就重新实例化一个error，把配置文件路径加上
		err = errors.New(file + ", " + err.Error())
	}
	return err
}

func defaultNodeConfig() node.Config {
	cfg := node.DefaultConfig
	// Name属性写死为geth
	cfg.Name = clientIdentifier
	cfg.Version = params.VersionWithCommit(gitCommit)
	// cfg.HTTPModules至此就包含"net", "web3", "eth", "shh"
	cfg.HTTPModules = append(cfg.HTTPModules, "eth", "shh")
	// cfg.WSModules至此就包含"net", "web3", "eth", "shh"
	cfg.WSModules = append(cfg.WSModules, "eth", "shh")
	cfg.IPCPath = "geth.ipc"
	return cfg
}

func makeConfigNode(ctx *cli.Context) (*node.Node, gethConfig) {
	// Load defaults.
	// 加载默认配置
	cfg := gethConfig{
		Eth:       eth.DefaultConfig,
		Shh:       whisper.DefaultConfig,
		Node:      defaultNodeConfig(),
		Dashboard: dashboard.DefaultConfig,
	}

	// Load config file.
	// 加载配置文件里的配置
	// 先找命令行参数--config对应的值，也就是配置文件的路径，如果不为空，就加载配置文件
	if file := ctx.GlobalString(configFileFlag.Name); file != "" {
		if err := loadConfig(file, &cfg); err != nil {
			// 打印错误到标准错误输出并且退出程序。
			// %v的语法参考：https://golang.org/pkg/fmt/
			utils.Fatalf("%v", err)
		}
	}

	// Apply flags.
	// 把命令行输入的flag应用到程序的配置中去
	utils.SetNodeConfig(ctx, &cfg.Node)
	stack, err := node.New(&cfg.Node)
	if err != nil {
		utils.Fatalf("Failed to create the protocol stack: %v", err)
	}
	utils.SetEthConfig(ctx, stack, &cfg.Eth)
	if ctx.GlobalIsSet(utils.EthStatsURLFlag.Name) {
		cfg.Ethstats.URL = ctx.GlobalString(utils.EthStatsURLFlag.Name)
	}

	utils.SetShhConfig(ctx, stack, &cfg.Shh)
	utils.SetDashboardConfig(ctx, &cfg.Dashboard)

	return stack, cfg
}

// enableWhisper returns true in case one of the whisper flags is set.
func enableWhisper(ctx *cli.Context) bool {
	for _, flag := range whisperFlags {
		if ctx.GlobalIsSet(flag.GetName()) {
			return true
		}
	}
	return false
}

func makeFullNode(ctx *cli.Context) *node.Node {
	stack, cfg := makeConfigNode(ctx)

	utils.RegisterEthService(stack, &cfg.Eth)

	if ctx.GlobalBool(utils.DashboardEnabledFlag.Name) {
		utils.RegisterDashboardService(stack, &cfg.Dashboard, gitCommit)
	}
	// Whisper must be explicitly enabled by specifying at least 1 whisper flag or in dev mode
	shhEnabled := enableWhisper(ctx)
	shhAutoEnabled := !ctx.GlobalIsSet(utils.WhisperEnabledFlag.Name) && ctx.GlobalIsSet(utils.DeveloperFlag.Name)
	if shhEnabled || shhAutoEnabled {
		if ctx.GlobalIsSet(utils.WhisperMaxMessageSizeFlag.Name) {
			cfg.Shh.MaxMessageSize = uint32(ctx.Int(utils.WhisperMaxMessageSizeFlag.Name))
		}
		if ctx.GlobalIsSet(utils.WhisperMinPOWFlag.Name) {
			cfg.Shh.MinimumAcceptedPOW = ctx.Float64(utils.WhisperMinPOWFlag.Name)
		}
		if ctx.GlobalIsSet(utils.WhisperRestrictConnectionBetweenLightClientsFlag.Name) {
			cfg.Shh.RestrictConnectionBetweenLightClients = true
		}
		utils.RegisterShhService(stack, &cfg.Shh)
	}

	// Add the Ethereum Stats daemon if requested.
	if cfg.Ethstats.URL != "" {
		utils.RegisterEthStatsService(stack, cfg.Ethstats.URL)
	}
	return stack
}

// dumpConfig is the dumpconfig command.
func dumpConfig(ctx *cli.Context) error {
	_, cfg := makeConfigNode(ctx)
	comment := ""

	if cfg.Eth.Genesis != nil {
		cfg.Eth.Genesis = nil
		comment += "# Note: this config doesn't contain the genesis block.\n\n"
	}

	out, err := tomlSettings.Marshal(&cfg)
	if err != nil {
		return err
	}
	io.WriteString(os.Stdout, comment)
	os.Stdout.Write(out)
	return nil
}
