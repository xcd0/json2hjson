package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/hjson/hjson-go/v4"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
)

// バージョン番号
var (
	Version  = "0.0.1"
	Revision = func() string { // {{{ ビルド時に埋め込まれたVCSの情報からgitのcommit IDを取得する。
		revision := ""
		modified := false
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					revision = setting.Value
					if len(setting.Value) > 7 {
						revision = setting.Value[:7] // 最初の7文字にする
					}
				}
				if setting.Key == "vcs.modified" {
					modified = setting.Value == "true"
				}
			}
		}
		if modified {
			revision = "develop+" + revision
		}
		return revision
	}() // }}}
)

func init() {
	log.SetFlags(log.Ltime | log.Lshortfile) // ログ出力の書式設定。
	log.SetOutput(io.Discard)                // --debug フラグが立っていないときログ出力を捨てる。
}

func main() {
	args, stdin := parseArgs() // 引数解析。
	indent := func() string {
		if args.Indent < 0 {
			return "\t"
		} else {
			return strings.Repeat(" ", args.Indent)
		}
	}()
	if len(stdin) > 0 {
		jsonToHjsonStdin(stdin, indent)
	}
	for _, jsonPath := range args.Files {
		if filepath.Ext(jsonPath) == ".json" {
			jsonToHjsonFile(jsonPath, indent)
		}
	}
}

// Args はコマンドライン引数を定義する構造体です
type Args struct {
	Files   []string `arg:"positional"   help:"JSON files to convert to Hjson"`
	Indent  int      `arg:"-i,--indent"  help:"インデント幅を指定。未指定の時tabを使用する。" placeholder:"W" default:"-1"`
	Version bool     `arg:"-v,--version" help:"print version number and exit"`
	Debug   bool     `arg:"-d,--debug"   help:"Enables detailed logging."`
}

func (args *Args) String() string {
	return fmt.Sprintf(`Args:
	Files:   %#v
	Indent:  %d
	Version: %t
	Debug:   %t`,
		strings.Join(args.Files, ", "), args.Indent, args.Version, args.Debug)
}

func parseArgs() (*Args, string) { // {{{ parseArgs は、コマンドライン引数を解析する関数です
	var parser *arg.Parser
	showHelp := func(post string) {
		buf := new(bytes.Buffer)
		parser.WriteHelp(buf)
		fmt.Printf("%v\n", strings.ReplaceAll(buf.String(), "display this help and exit", "ヘルプを出力する。"))
		if len(post) != 0 {
			fmt.Println(post)
		}
		os.Exit(1)
	}
	programName := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	showVersion := func() {
		if len(Revision) == 0 {
			// go installでビルドされた場合、gitの情報がなくなる。その場合v0.0.0.のように末尾に.がついてしまうのを避ける。
			fmt.Printf("%v version %v\n", programName, Version)
		} else {
			fmt.Printf("%v version %v.%v\n", programName, Version, Revision)
		}
		os.Exit(0)
	}
	args := &Args{}
	var err error

	parser, err = arg.NewParser(arg.Config{Program: programName, IgnoreEnv: false}, args)
	if err != nil {
		showHelp(fmt.Sprintf("%v", errors.Errorf("%v", err)))
	}
	if err := parser.Parse(os.Args[1:]); err != nil {
		if err.Error() == "help requested by user" {
			showHelp("")
		} else if err.Error() == "version requested by user" {
			showVersion()
		} else {
			panic(errors.Errorf("%v", err))
		}
	}
	if args.Debug {
		log.SetOutput(os.Stderr)
		log.Println(args)
	}
	if args.Version {
		showVersion()
		os.Exit(0)
	}
	if len(args.Files) == 0 {
		if str := GetStringFromStdin(); len(str) > 0 {
			return args, str
		} else {
			showHelp("")
			os.Exit(0)
		}
	}
	return args, ""
} // }}}

func jsonToHjsonFile(jsonPath string, indent string) { // {{{ jsonToHjsonFile は、JSONファイルを読み込み、Hjsonファイルとして出力する関数です

	log.Printf("jsonToHjsonFile: indent: %#q", indent)

	// JSONファイルを読み込む
	jsonData, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		panic(errors.Errorf("%v", err))
	}

	// JSONをパースしてMapに変換
	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		panic(errors.Errorf("%v", err))
	}

	// Hjsonに変換
	option := hjson.DefaultOptions()
	option.IndentBy = indent
	option.Eol = "\n"
	hjsonData, err := hjson.MarshalWithOptions(data, option)
	if err != nil {
		panic(errors.Errorf("%v", err))
	}

	// 出力ファイル名を決定（拡張子を.jsonから.hjsonに変更）
	hjsonPath := jsonPath[:len(jsonPath)-len(filepath.Ext(jsonPath))] + ".hjson"

	// Hjsonファイルに書き込む
	if err := ioutil.WriteFile(hjsonPath, hjsonData, 0644); err != nil {
		panic(errors.Errorf("%v", err))
	}

	fmt.Printf("Converted %s to %s\n", jsonPath, hjsonPath)
	log.Printf("Converted: \n%v", string(hjsonData))
} // }}}

func jsonToHjsonStdin(jsonData, indent string) { // {{{ jsonToHjsonStdin は、標準入力からJSONを読み込み、標準出力にHjsonを出力する関数です

	log.Printf("jsonToHjsonFile: indent: %#q", indent)

	// JSONをパースしてMapに変換
	var data interface{}
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		log.Println("Input is not valid JSON")
		return
	}

	// Hjsonに変換して出力
	option := hjson.DefaultOptions()
	option.IndentBy = indent
	option.Eol = "\n"
	hjsonData, err := hjson.MarshalWithOptions(data, option)
	if err != nil {
		panic(errors.Errorf("%v", err))
	}

	log.Printf("Converted:")
	fmt.Println(string(hjsonData))
} // }}}

func GetStringFromStdin() string { // {{{ isInputFromStdin は、標準入力からの入力があるかどうかを判定する関数です
	if terminal.IsTerminal(0) {
		return ""
	}
	b, _ := ioutil.ReadAll(os.Stdin)
	log.Printf("stdin:%#v\n", string(b))
	return string(b)
} // }}}
