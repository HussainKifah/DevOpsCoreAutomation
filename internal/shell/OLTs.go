package shell

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type OLT struct {
	Ip     string `json:"ip"`
	Name   string `json:"name"`
	Site   string `json:"site"`
	Vendor string `json:"vendor"`
}
type OLTs []OLT

func OLTsData() (nokia OLTs, huawei OLTs, err error) {
	apiURL := os.Getenv("OLTS_API_ENV")
	if apiURL == "" {
		return nil, nil, fmt.Errorf("OLTS_API_ENV not set")
	}
	res, err := http.Get(apiURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch OLT data: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read OLT API response: %w", err)
	}

	var data OLTs
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, nil, fmt.Errorf("parse OLT API response: %w", err)
	}

	for _, olt := range data {
		if olt.Ip == "" {
			continue
		}

		if strings.HasPrefix(olt.Ip, "10.90.3.") ||
			olt.Ip == "10.250.0.178" ||
			olt.Ip == "10.202.160.3" ||
			olt.Ip == "10.80.2.161" {
			huawei = append(huawei, olt)
		} else {
			nokia = append(nokia, olt)
		}
	}

	return nokia, huawei, nil
}

// GetHuaweiOLTs returns Huawei OLTs from OLTsData (OLTs_API_ENV) when available, else from HW_OLT_HOSTS or default.
func GetHuaweiOLTs() []OLT {
	_, hw, err := OLTsData()
	if err == nil && len(hw) > 0 {
		return hw
	}
	hosts := os.Getenv("HW_OLT_HOSTS")
	if hosts == "" {
		hosts = "10.250.0.178,10.202.160.3,10.90.3.101,10.90.3.102,10.90.3.103,10.90.3.104,10.80.2.161"
	}
	var out []OLT
	for _, ip := range strings.Split(hosts, ",") {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		out = append(out, OLT{Ip: ip, Name: "OLT-" + strings.ReplaceAll(ip, ".", "-"), Site: "site", Vendor: "huawei"})
	}
	return out
}
