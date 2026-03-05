// {"ip":"10.250.0.178","oltType":"huawei"},
// {"ip":"10.202.160.3","oltType":"huawei"},
// {"ip":"10.90.3.101","oltType":"huawei"},
// {"ip":"10.90.3.102""oltType":"huawei"},
// {"ip":"10.90.3.103","oltType":"huawei"},
// {"ip":"10.90.3.104","oltType":"huawei"},
// {"ip":"10.80.2.161","oltType":"huawei"},

package main

import (
	"fmt"
	"log"
	"regexp"
	"time"

	expect "github.com/google/goexpect"
	"golang.org/x/crypto/ssh"
)

func main() {
	config := &ssh.ClientConfig{
		User: "devops",
		Auth: []ssh.AuthMethod{
			ssh.Password(""),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Config: ssh.Config{
			KeyExchanges: []string{"diffie-hellman-group1-sha1"},
			Ciphers:      []string{"aes128-cbc", "3des-cbc"},
		},
		HostKeyAlgorithms: []string{"ssh-rsa"},
	}
	client, err := ssh.Dial("tcp", "10.250.0.178:22", config)
	if err != nil {
		log.Fatalf("SSH failed: %v", err)
	}
	prompt := regexp.MustCompile(`\(config-if-gpon-0/1\)#`)
	e, _, _ := expect.SpawnSSH(client, time.Second*10)
	var pons []string
	e.Expect(regexp.MustCompile(`>`), time.Second*5)
	e.Send("enable\n")
	e.Expect(regexp.MustCompile(`MA5600T#`), time.Second*5)
	e.Send("config\n")
	e.Expect(regexp.MustCompile(`\(config\)#`), time.Second*5)
	e.Send("interface gpon 0/1\n")
	e.Expect(prompt, time.Second*5)
	for i := 0; i < 10; i++ {
		e.Send(fmt.Sprintf("display ont optical-info 0 %d\n", i))
		pon, _, _ := e.Expect(prompt, time.Second*15)
		pons = append(pons, pon)
	}
	fmt.Println(pons)

}
