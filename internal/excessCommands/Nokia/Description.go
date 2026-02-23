package excesscommands

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/utils"
)



type ontPower struct {
	OntIdx string `json:"OntIdx"`
	OltRx float64 `json:"OltRx"`
}
type HostResult struct {
	Host string     `json:"Host"`
	Data []ontPower `json:"Data"`
	Err any         `json:"Err"`
}

type OntResult struct {
	OntIdx string  `json:"ont_idx"`
	OltRx  float64 `json:"olt_rx"`
	Desc1  string  `json:"desc1"`
	Desc2  string  `json:"desc2"`
}

type CmdResult struct {
	Host   string      `json:"host"`
	Output []OntResult `json:"output"`
}

type FlatDesc struct {
	Host   string  `json:"host"`
	OntIdx string  `json:"ont_idx"`
	OltRx  float64 `json:"olt_rx"`
	Desc1  string  `json:"desc1"`
	Desc2  string  `json:"desc2"`
}


func ExtractOntIDAndDesc(username, password string) []CmdResult {
	b, err := os.ReadFile("/home/hussain/Desktop/DevOpsCoreAutomation/cmd/json/all-onts-powers-less-than-24.json")
	if err != nil {
		panic(err)
	}
	var results []HostResult
	if err := json.Unmarshal(b, &results); err != nil {
		panic(err)
	}
	resultsCh := make(chan CmdResult, len(results))
	var wg sync.WaitGroup
	parallelSessions := make(chan struct{}, 50)

	for _, r := range results {
		if len(r.Data) == 0 {
			continue
		}

		powerByIdx := make(map[string]float64, len(r.Data))
		for _, p := range r.Data {
			powerByIdx[p.OntIdx] = p.OltRx
		}

		host := r.Host
		wg.Add(1)

		go func(host string, powers map[string]float64) {
			defer wg.Done()

			parallelSessions <- struct{}{}
			defer func() { <-parallelSessions }()

			output, err := shell.NkSendCommandOLT(host, username, password, "show equipment ont status pon")
			if err != nil {
				log.Printf("ERROR: %s: %v", host, err)
				return
			}
			allDescs := extractor.ExtractAllDesc(output)
			var filtered []OntResult

			for _, d := range allDescs {
				if rx, ok := powers[d.OntIdx]; ok {
					filtered = append(filtered, OntResult{
						OntIdx: d.OntIdx,
						OltRx:  rx,
						Desc1:  d.Desc1,
						Desc2:  d.Desc2,
					})
				}
			}
			if len(filtered) > 0 {
				resultsCh <- CmdResult{Host: host, Output: filtered}
			}
		}(host, powerByIdx)
	}
	go func ()  {
		wg.Wait()
		close(resultsCh)
	}()
	var wholeResult []CmdResult
	for w := range resultsCh {
		wholeResult = append(wholeResult, w)
	}

	var rows []FlatDesc
for _, r := range wholeResult {
	for _, d := range r.Output {
		rows = append(rows, FlatDesc{
			Host:   r.Host,
			OntIdx: d.OntIdx,
			OltRx:  d.OltRx,
			Desc1:  d.Desc1,
			Desc2:  d.Desc2,
		})
	}
}

xlPath, err := utils.SaveExcel("json", "out-status-results", rows)
if err != nil {
	log.Fatal(err)
}
fmt.Println("saved:", xlPath)

	// path, err := utils.SaveJSON("json", "out-status-results", wholeResult)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println("saved:", path)

	return wholeResult
}