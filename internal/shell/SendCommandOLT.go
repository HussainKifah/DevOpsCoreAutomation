package shell

import (
	"regexp"
	"sync"
	"time"

	"github.com/scrapli/scrapligo/driver/generic"
	"github.com/scrapli/scrapligo/driver/options"
	"github.com/scrapli/scrapligo/transport"
)

type Result struct {
	Device string
	Site   string
	Host   string
	Data   string
	Err    error
}

func NkSendCommandOLT(host, user, pass string, cmds ...string) (string, error) {

	// init new device
	driver, err := generic.NewDriver(
		host,
		options.WithAuthNoStrictKey(),
		options.WithAuthUsername(user),
		options.WithAuthPassword(pass),
		// options.WithChannelLog(os.Stdout),
		// options.WithOnOpen(func(d *generic.Driver) error {
		// 	_,_ = d.SendCommand("screen-length 0 temporary")
		// 	return nil
		// }),
		options.WithPromptPattern(regexp.MustCompile(`(?m)(>#)\s*$`)),
		options.WithTransportType(transport.StandardTransport),
		options.WithSSHConfigFile(""),
		options.WithTimeoutOps(120*time.Minute),
		options.WithTermWidth(511),
	)
	if err != nil {
		return "", err
	}

	//openning a session
	if err := driver.Open(); err != nil {
		return "", err
	}
	defer driver.Close()

	if len(cmds) == 1 {
		r, err := driver.SendCommand(cmds[0])
		if err != nil {
			return "", err
		}
		return r.Result, err

	} else {
		rs, err := driver.SendCommands(cmds)
		if err != nil {
			return "", err
		}
		return rs.JoinedResult(), err
	}

}

func HwSendCommandOLT(host, user, pass string, cmds ...string) (string, error) {
	driver, err := generic.NewDriver(
		host,
		options.WithAuthNoStrictKey(),
		options.WithAuthUsername(user),
		options.WithAuthPassword(pass),
		options.WithPromptPattern(regexp.MustCompile(`(?m)[<>]\S+[<>]\s*$`)),
		options.WithTransportType(transport.StandardTransport),
		options.WithSSHConfigFile(""),
		options.WithTimeoutOps(90*time.Minute),
		options.WithTermWidth(511),
		options.WithOnOpen(func(d *generic.Driver) error {
			_, _ = d.SendCommand("screen-length 0 temporary")
			return nil
		}),
	)
	if err != nil {
		return "", err
	}

	if err := driver.Open(); err != nil {
		return "", err
	}
	defer driver.Close()

	if len(cmds) == 1 {
		r, err := driver.SendCommand(cmds[0])
		if err != nil {
			return "", err
		}
		return r.Result, err
	} else {
		rs, err := driver.SendCommands(cmds)
		if err != nil {
			return "", err
		}
		return rs.JoinedResult(), err
	}
}

func SendCommandNokiaOLTs(username, password string, cmds ...string) <-chan Result {
	nokia, _ := OLTsData()
	results := make(chan Result, len(nokia))
	var wg sync.WaitGroup

	parallelSessions := make(chan struct{}, 33)

	for _, olt := range nokia {
		olt := olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			parallelSessions <- struct{}{}
			defer func() { <-parallelSessions }()

			data, err := NkSendCommandOLT(olt.Ip, username, password, cmds...)
			results <- Result{Device: olt.Name, Site: olt.Site, Host: olt.Ip, Data: data, Err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

func SendCommandHuaweiOLTs(username, password string, cmds ...string) <-chan Result {
	_, huawei := OLTsData()
	results := make(chan Result, len(huawei))
	var wg sync.WaitGroup
	parallelSessions := make(chan struct{}, 33)

	for _, olt := range huawei {
		olt := olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			parallelSessions <- struct{}{}
			defer func() { <-parallelSessions }()

			data, err := HwSendCommandOLT(olt.Ip, username, password, cmds...)
			results <- Result{Device: olt.Name, Site: olt.Site, Host: olt.Ip, Data: data, Err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

func SendCommandAllOlTs(username, password, nokiaCmd, huaweiCmd string) <-chan Result {

	merged := make(chan Result)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for r := range SendCommandNokiaOLTs(username, password, nokiaCmd) {
			merged <- r
		}
	}()
	go func() {
		defer wg.Done()
		for r := range SendCommandHuaweiOLTs(username, password, huaweiCmd) {
			merged <- r
		}
	}()

	go func() {
		wg.Wait()
		close(merged)
	}()
	return merged
}
