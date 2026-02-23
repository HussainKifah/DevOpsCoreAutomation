package shell

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type OLT struct {
	Ip     string `json:"ip"`
	Name   string `json:"name"`
	Site   string `json:"site"`
	Vendor string `json:"vendor"`
}
type OLTs []OLT

func OLTsData() (nokia OLTs, huawei OLTs) {
	_ = godotenv.Load("../.env")

	res, err := http.Get(os.Getenv("OLTS_API_ENV"))
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}

	var data OLTs
	if err := json.Unmarshal(body, &data); err != nil {
		panic(err)
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

	return nokia, huawei
}
