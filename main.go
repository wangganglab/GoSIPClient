package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwebrtc/go-sip-ua/examples/mock"
	"github.com/cloudwebrtc/go-sip-ua/pkg/account"
	"github.com/cloudwebrtc/go-sip-ua/pkg/media/rtp"
	"github.com/cloudwebrtc/go-sip-ua/pkg/session"
	"github.com/cloudwebrtc/go-sip-ua/pkg/stack"
	"github.com/cloudwebrtc/go-sip-ua/pkg/ua"

	"github.com/cloudwebrtc/go-sip-ua/pkg/utils"
	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/sip/parser"
)

var (
	logger log.Logger
	udp    *rtp.RtpUDPStream
)

func init() {
	logger = utils.NewLogrusLogger(log.DebugLevel, "Client", nil)
	// logger = log.NewDefaultLogrusLogger().WithPrefix("Client")
}

func createUdp() *rtp.RtpUDPStream {

	udp = rtp.NewRtpUDPStream("198.19.249.3", rtp.DefaultPortMin, rtp.DefaultPortMax, func(data []byte, raddr net.Addr) {
		logger.Infof("Rtp recevied: %v, laddr %s : raddr %s", len(data), udp.LocalAddr().String(), raddr)
		dest, _ := net.ResolveUDPAddr(raddr.Network(), raddr.String())
		logger.Infof("Echo rtp to %v", raddr)
		udp.Send(data, dest)
	})

	go udp.Read()

	return udp
}

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	stack := stack.NewSipStack(&stack.SipStackConfig{
		UserAgent:  "Go Sip Client/example-client",
		Extensions: []string{"replaces", "outbound"},
		Dns:        "8.8.8.8"})

	listen := "0.0.0.0:5080"
	logger.Infof("Listen => %s", listen)

	if err := stack.Listen("udp", listen); err != nil {
		logger.Panic(err)
	}

	//if err := stack.Listen("tcp", listen); err != nil {
	//	logger.Panic(err)
	//}
	//
	//if err := stack.ListenTLS("wss", "0.0.0.0:5091", nil); err != nil {
	//	logger.Panic(err)
	//}

	ua := ua.NewUserAgent(&ua.UserAgentConfig{
		SipStack: stack,
	})

	ua.InviteStateHandler = func(sess *session.Session, req *sip.Request, resp *sip.Response, state session.Status) {
		logger.Infof("InviteStateHandler: state => %v, type => %s", state, sess.Direction())

		switch state {
		case session.InviteReceived:
			udp = createUdp()
			udpLaddr := udp.LocalAddr()
			sdp := mock.BuildLocalSdp(udpLaddr.IP.String(), udpLaddr.Port)
			sess.ProvideAnswer(sdp)
			sess.Accept(200)
		case session.Canceled:
			fallthrough
		case session.Failure:
			fallthrough
		case session.Terminated:
			udp.Close()
		}
	}

	ua.RegisterStateHandler = func(state account.RegisterState) {
		logger.Infof("RegisterStateHandler: user => %s, state => %v, expires => %v", state.Account.AuthInfo.AuthUser, state.StatusCode, state.Expiration)
	}

	uri, err := parser.ParseUri("sip:1002@198.19.249.171")
	if err != nil {
		logger.Error(err)
	}

	profile := account.NewProfile(uri.Clone(), "goSIP/example-client",
		&account.AuthInfo{
			AuthUser: "1002",
			Password: "1234",
			Realm:    "",
		},
		1800,
		stack,
	)

	recipient, err := parser.ParseSipUri("sip:1002@198.19.249.171;transport=udp")
	if err != nil {
		logger.Error(err)
	}

	register, _ := ua.SendRegister(profile, recipient, profile.Expires, nil)
	time.Sleep(time.Second * 3)

	udp = createUdp()
	udpLaddr := udp.LocalAddr()
	sdp := mock.BuildLocalSdp(udpLaddr.IP.String(), udpLaddr.Port)

	called, err2 := parser.ParseUri("sip:9002@198.19.249.171")
	if err2 != nil {
		logger.Error(err)
	}

	recipient, err = parser.ParseSipUri("sip:9002@198.19.249.171;transport=udp")
	if err != nil {
		logger.Error(err)
	}

	go ua.Invite(profile, called, recipient, &sdp)

	<-stop

	register.SendRegister(0)

	ua.Shutdown()
}
