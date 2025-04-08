package main

import (
	"code.cloudfoundry.org/tlsconfig"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-shellwords"
	"github.com/nxsre/go-rsh"
	"github.com/nxsre/go-rsh/pb"
	"io"
	"log"
	"net"
	"os"
	"time"
)

var (
	port   = flag.Uint("p", 22222, "server port")
	addr   = flag.String("a", "127.0.0.1", "server address")
	cacert = flag.String("ca", "./certs/ca.pem", "ca certificate file")
	cert   = flag.String("cert", "./certs/server.pem", "server certificate file")
	key    = flag.String("key", "./certs/server-key.pem", "server key file")
)

func parseArgs() {
	flag.Parse()

	if port == nil || *port == 0 {
		log.Fatal("-p is required")
	}

	if *port > 65535 {
		log.Fatal("Invalid port: ")
	}

	if addr == nil || *addr == "" {
		log.Fatal("-a is required")
	}
}

func main() {
	parseArgs()
	log.Printf("Serving at %s:%d", *addr, *port)

	tlscfg, err := tlsconfig.Build(
		tlsconfig.WithIdentityFromFile(*cert, *key),
	).Server(tlsconfig.WithClientAuthenticationFromFile(*cacert))
	if err != nil {
		log.Fatal(err)
	}

	tlscfg.ClientAuth = tls.VerifyClientCertIfGiven

	// 禁用控制台颜色
	gin.DisableConsoleColor()
	router := gin.Default()
	router.Use(rsh.Recover)

	//处理找不到路由
	router.NoRoute(rsh.HandleNotFound)
	router.NoMethod(rsh.HandleNotFound)

	server := rsh.NewReverseServer(router, tlscfg)
	server.RegisterHandlers()

	router.GET("/get/:deviceId", NewWeb(server))

	nl, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *addr, *port))
	if err != nil {
		log.Fatalln(err)
	}

	h2Config := tlscfg.Clone()
	h2Config.NextProtos = rsh.DeDuplicateSlice(append(h2Config.NextProtos, "h2")) // grpc 要求 ALPN 支持
	nl = tls.NewListener(nl, h2Config)

	if err := router.RunListener(nl); err != nil {
		log.Fatalln(err)
	}
}

func NewWeb(server *rsh.ReverseServer) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("deviceID:::", c.Param("deviceId"))
		channel := server.GetClient(c.Param("deviceId"))
		if channel == nil {
			log.Println("channel not found")
			rsh.NewResult(c).ErrorCode(404, "资源未找到", nil)
			return
		}

		opts := &rsh.ExecOptions{
			Command: "ls",
			Args:    []string{"/"},
		}

		if cmd, ok := c.GetQuery("cmd"); ok {
			args, err := shellwords.Parse(cmd)
			if err != nil {
				rsh.NewResult(c).ErrorCode(100, "解析错误", err)
				return
			}
			opts.Command = args[0]
			opts.Args = args[1:]
		}

		// let's ask some stuff
		client := pb.NewRemoteShellClient(channel)

		// Contact the server and print out its response.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// 调用客户端的服务
		// stream  标准输出流，参考 Client 的相关方法使用 stream
		stream, err := client.Session(ctx)
		if err != nil {
			log.Printf("start shell session: %v", err)
			return
		}

		log.Println("DEBUG", "ExecOpts", opts)

		err = stream.Send(&pb.Input{
			Start:   true,
			Command: opts.Command,
			Args:    opts.Args,
		})
		if err != nil {
			log.Printf("send cmd: %v", err)
			return
		}

		var (
			inc  = make(chan rune, 1024)
			sigc = make(chan os.Signal, 1)
		)

		// TODO: 需要终端的场景(比如 webterm)
		if opts.Terminal {
			go rsh.WriteStream(stream, inc, sigc)
		}

		stdoutR, stdoutW := io.Pipe()
		stderrR, stderrW := io.Pipe()
		// 合并 stdout stderr 到 c.Writer
		go io.Copy(c.Writer, stderrR)
		go io.Copy(c.Writer, stdoutR)

		exitCode, err := rsh.ReadStream(stream, stdoutW, stderrW)
		if exitCode != nil {
			fmt.Printf("\n状态码: %v err: %v\n", *exitCode, err)
		}
	}
}
