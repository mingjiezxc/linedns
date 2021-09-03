package main

import (
	"context"
	"io/ioutil"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/domainr/dnsr"
	"github.com/miekg/dns"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v2"
)

var (
	cli       *clientv3.Client
	appConfig YamlConfig
	noteRe, _ = regexp.Compile("^;")
)

type YamlConfig struct {
	ListeningAddr string   `yaml:"ListeningAddr"`
	ListeningPort string   `yaml:"ListeningPort"`
	ZoneName      string   `yaml:"ZoneName"`
	LineName      string   `yaml:"LineName"`
	EtcdServers   []string `yaml:"EtcdServers"`
	EtcdUser      string   `yaml:"EtcdUser"`
	EtcdPassword  string   `yaml:"EtcdPassword"`
}

func main() {
	go CliRegedit()
	defer cli.Close()
	QueryListeningServerStart()
}

func init() {
	// read config file
	configfile, err := ioutil.ReadFile("./config.yaml")
	if ErrCheck(err) {
		os.Exit(1)
	}

	// yaml marshal config
	err = yaml.Unmarshal(configfile, &appConfig)
	if ErrCheck(err) {
		os.Exit(2)
	}

	cli, err = clientv3.New(clientv3.Config{
		Endpoints:   appConfig.EtcdServers,
		Username:    appConfig.EtcdUser,
		Password:    appConfig.EtcdPassword,
		DialTimeout: 10 * time.Second,
	})
	if ErrCheck(err) {
		os.Exit(3)
	}
}

func CliRegedit() {
	for {
		resp, err := cli.Grant(context.TODO(), 5)
		if ErrCheck(err) {
			continue
		}

		key := "/line/dns/" + appConfig.ZoneName + "/" + appConfig.LineName + "/" + appConfig.ListeningAddr + ":" + appConfig.ListeningPort

		_, err = cli.Put(context.TODO(), key, "online", clientv3.WithLease(resp.ID))
		if ErrCheck(err) {
			continue
		}
		// to renew the lease only once
		_, err = cli.KeepAlive(context.TODO(), resp.ID)
		if ErrCheck(err) {
			continue
		}

		break
	}
	log.Println("Etcd Regedit Config done")

}

func QueryListeningServerStart() {
	p := make([]byte, 2000)

	port, err := strconv.Atoi(appConfig.ListeningPort)
	if ErrCheck(err) {
		os.Exit(5)
	}

	addr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP("0.0.0.0"),
	}
	ser, err := net.ListenUDP("udp", &addr)
	if ErrCheck(err) {
		os.Exit(5)
	}

	for {
		cnt, addr, err := ser.ReadFromUDP(p)
		if err != nil {
			continue
		}

		go DnsQueryQuestion(addr.IP.String(), p[0:cnt])
	}
}

func DnsQueryQuestion(addr string, data []byte) {
	// unMarshal data
	query := &DnsQuery{}
	err := proto.Unmarshal(data, query)
	if err != nil {
		return
	}

	if query.Tdns == "0.0.0.0" {
		IterativeQuery(query)
	} else {
		err = RecursiveQuery(query)
		if err != nil {
			return
		}
	}

	c, err := net.Dial("udp4", addr+":"+query.Tport)
	if err != nil {
		return
	}
	pData, err := proto.Marshal(query)
	if err != nil {
		return
	}
	c.Write(pData)

}

func RecursiveQuery(query *DnsQuery) error {
	m1 := new(dns.Msg)
	m1.Id = dns.Id()
	m1.RecursionDesired = true
	m1.Question = make([]dns.Question, 1)
	m1.Question[0] = dns.Question{
		Name:   query.Domain,
		Qtype:  uint16(query.Dnstype),
		Qclass: uint16(query.Dnsclass),
	}

	dc := new(dns.Client)
	in, _, err := dc.Exchange(m1, query.Tdns+":53")
	if err != nil {
		return err
	}

	lineStr := strings.Split(in.String(), "\n")

	for _, s := range lineStr {

		if noteRe.MatchString(s) && s == "" {
			continue
		}
		query.Rr = append(query.Rr, s)

	}
	return nil

}

func IterativeQuery(query *DnsQuery) {
	r := dnsr.New(5000)
	for _, rr := range r.Resolve(query.Domain, dns.Type(query.Dnstype).String()) {
		query.Rr = append(query.Rr, rr.String())
	}
}

func StrToUint16(s string) uint16 {
	value, _ := strconv.ParseUint(s, 16, 16)
	return uint16(value) // done!
}

func ErrCheck(err error) bool {
	if err != nil {
		log.Println(err.Error())
		return true
	}
	return false
}
