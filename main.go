package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/gin-gonic/gin"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v2"
)

var (
	cli       *clientv3.Client
	appConfig YamlConfig
)

type YamlConfig struct {
	ListeningAddr string
	ListeningPort string
	ZoneName      string
	LineName      string
	EtcdServers   []string
	EtcdUser      string
	EtcdPassword  string
}

func main() {
	go CliRegedit()
	defer cli.Close()

	r := gin.Default()
	r.GET("/query/:domain/:type/:class/:dns", DnsQuery)

	r.Run("0.0.0.0:" + appConfig.ListeningPort) // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
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

		key := "/line/dns/" + appConfig.ZoneName + "/" + appConfig.LineName + "/" + appConfig.ListeningAddr + appConfig.ListeningPort

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
	log.Println("Etcd Regedit Config done:", key)

}

func DnsQuery(c *gin.Context) {
	dnsType := StrToUint16(c.Param("type"))
	dnsClass := StrToUint16(c.Param("class"))
	domain := c.Param("domain")
	dnsServer := c.Param("dns")

	m1 := new(dns.Msg)
	m1.Id = dns.Id()
	m1.RecursionDesired = true
	m1.Question = make([]dns.Question, 1)
	m1.Question[0] = dns.Question{domain, dnsType, dnsClass}

	dc := new(dns.Client)
	in, _, err := dc.Exchange(m1, dnsServer+":53")
	if err != nil {
		c.String(201, err)
	} else {
		c.String(200, in.String())

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
