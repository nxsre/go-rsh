package rsh

import (
	"crypto/tls"
	"github.com/alphadose/haxmap"
	"github.com/bytedance/gopkg/cloud/metainfo"
	"github.com/gin-gonic/gin"
	"github.com/jhump/grpctunnel"
	"github.com/jhump/grpctunnel/tunnelpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"log"
	"strings"
)

type ReverseServer struct {
	tlsconfig *tls.Config
	clients   *haxmap.Map[string, grpc.ClientConnInterface]
	router    *gin.Engine
}

// Reverse client. 集成在客户端的反向 shell(用于 grpc server 端调用 agent 侧 shell)
func NewReverseServer(router *gin.Engine, tlscfg *tls.Config) *ReverseServer {
	s := &ReverseServer{
		tlsconfig: tlscfg,
		router:    router,
		clients:   haxmap.New[string, grpc.ClientConnInterface](),
	}
	return s
}

func (s *ReverseServer) GetClient(id string) grpc.ClientConnInterface {
	for k, v := range s.clients.Iterator() {
		log.Println(k, v)
	}
	conn, ok := s.clients.Get(id)
	if !ok {
		return nil
	}
	return conn
}

func (s *ReverseServer) RegisterHandlers() {
	log.Println("server started")
	handler := grpctunnel.NewTunnelServiceHandler(
		grpctunnel.TunnelServiceHandlerOptions{
			OnReverseTunnelOpen: func(channel grpctunnel.TunnelChannel) {
				log.Println("tunnel opened,new client connect...")
				// 获取客户端信息,身份验证阶段
				peerInfo, ok := peer.FromContext(channel.Context())
				if ok && peerInfo.AuthInfo != nil {
					tlsInfo := peerInfo.AuthInfo.(credentials.TLSInfo)
					for _, v := range tlsInfo.State.PeerCertificates {
						//fmt.Println("Client: Server public key is:")
						//fmt.Println(x509.MarshalPKIXPublicKey(v.PublicKey))
						log.Println("client cert cn:", v.Subject.CommonName)
						if v.Subject.CommonName != "root" {
							channel.Close()
							return
						}
					}
				}

				log.Println(metainfo.GetAllValues(channel.Context()))
				log.Println("New Tunnel Opened", peerInfo.String(), ok)
				md, ok := metadata.FromIncomingContext(channel.Context())
				log.Println("New Tunnel Metadata", md, ok)

				if k := md.Get("rpc-transit-client-id"); len(k) > 0 {
					log.Println("PROXY --- 新客户端:", k)
					s.clients.Set(k[0], channel)
				}
				if k := md.Get("client-id"); len(k) > 0 {
					log.Println("新客户端:", k)
					s.clients.Set(k[0], channel)
				}
			},
			OnReverseTunnelClose: func(channel grpctunnel.TunnelChannel) {
				log.Println("Tunnel Closed")
				// 获取客户端信息,身份验证阶段
				peer, ok := peer.FromContext(channel.Context())
				if ok {

				}
				log.Println("### Tunnel Closed", peer.String(), ok)
				md, ok := metadata.FromIncomingContext(channel.Context())
				log.Println("### Tunnel Metadata", md, ok)

				if k := md.Get("client-id"); len(k) > 0 {
					s.clients.Del(k[0])
				}
			},
		},
	)

	// TLS认证
	creds := credentials.NewTLS(s.tlsconfig)
	svr := grpc.NewServer(grpc.Creds(creds))

	tunnelpb.RegisterTunnelServiceServer(svr, handler.Service())
	tunnelGroup := s.router.Group("/grpctunnel.v1.TunnelService")

	tunnelGroup.Any("/*name", func(c *gin.Context) {
		_ = c.Param("name")
		log.Println("请求路径:", c.Request.URL)
		// 判断是否 grpc 请求，如果是 grpc 请求，由 grpc server 处理
		if c.Request.ProtoMajor == 2 && strings.Contains(c.Request.Header.Get("Content-Type"), "application/grpc") {
			svr.ServeHTTP(c.Writer, c.Request)
		} else {
			log.Println("aaaa")
		}
	})
}

// 数组去重
func DeDuplicateSlice[T any](array []T) []T {
	mp := make(map[any]struct{})
	idx := 0
	for _, value := range array {
		if _, ok := mp[value]; ok {
			continue
		}
		array[idx] = value
		idx = idx + 1
		mp[value] = struct{}{}
	}
	return array[:idx]
}
