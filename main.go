package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/gin-gonic/gin"
	"github.com/miekg/dns"
)

var (
	cli       *clientv3.Client
	agentName = "localhost"
	zoneName  = "gz"
	lineName  = "xx"
)

func main() {
	go CliInit()
	defer cli.Close()

	r := gin.Default()
	r.GET("/query/:domain/:type/:class/:dns", DnsQuery)

	r.Run("0.0.0.0:8445") // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}

func CliInit() {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := cli.Grant(context.TODO(), 5)
	if err != nil {
		log.Fatal(err)
	}

	key := "/line/dns/" + zoneName + "/" + lineName + "/" + agentName + ":8445"

	_, err = cli.Put(context.TODO(), key, "online", clientv3.WithLease(resp.ID))
	if err != nil {
		log.Fatal(err)
	}

	// to renew the lease only once
	_, kaerr := cli.KeepAlive(context.TODO(), resp.ID)
	if kaerr != nil {
		log.Fatal(kaerr)
	}

	log.Println("etcd keep alive Start:", key)

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
	in, _, _ := dc.Exchange(m1, dnsServer+":53")

	c.String(200, in.String())
}

func StrToUint16(s string) uint16 {
	value, _ := strconv.ParseUint(s, 16, 16)
	return uint16(value) // done!
}
