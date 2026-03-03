package excesscommands

import (
	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
)

type InventorySummary struct {
	Counts      []extractor.EquipIDCount `json:"counts"`
	VendorCount []extractor.VendorCount  `jsonl:"vender_counts"`
	Total       int                      `json:"total"`
}

type OLTInventory struct {
	Host         string                   `json:"host"`
	Device       string                   `json:"device"`
	Site         string                   `json:"site"`
	Counts       []extractor.EquipIDCount `json:"counts"`
	Vendorcounts []extractor.VendorCount  `json:"vender_counts"`
	Total        int                      `json:"total"`
}

func TotalInventory(pool *shell.ConnectionPool) InventorySummary {
	cmd := "show equipment ont interface detail | match exact:equip-id | count"
	totals := make(map[string]int)
	var order []string

	for olt := range shell.SendCommandNokiaOLTsPooled(pool, cmd) {
		for _, c := range extractor.CountEquipIDs(olt.Data) {

			if totals[c.ID] == 0 {
				order = append(order, c.ID)
			}
			totals[c.ID] += c.Count
		}
	}
	counts := make([]extractor.EquipIDCount, 0, len(order))
	total := 0
	vendorTotals := make(map[string]int)
	var vendorOrder []string

	for _, id := range order {
		c := totals[id]
		counts = append(counts, extractor.EquipIDCount{ID: id, Count: totals[id]})
		total += c

		vendor := extractor.GetVender(id)
		if vendorTotals[vendor] == 0 {
			vendorOrder = append(vendorOrder, vendor)
		}
		vendorTotals[vendor] += c
	}
	venderCounts := make([]extractor.VendorCount, 0, len(vendorOrder))
	for _, v := range vendorOrder {
		venderCounts = append(venderCounts, extractor.VendorCount{Vendor: v, Count: vendorTotals[v]})
	}
	return InventorySummary{Counts: counts, VendorCount: venderCounts, Total: total}
}

func InventoryPerOLT(pool *shell.ConnectionPool) []OLTInventory {
	cmd := "show equipment ont interface detail | match exact:equip-id | count"
	var results []OLTInventory

	for olt := range shell.SendCommandNokiaOLTsPooled(pool, cmd) {
		counts := extractor.CountEquipIDs(olt.Data)
		total := 0
		vendorTotals := make(map[string]int)
		var vendorOrder []string

		for _, c := range counts {
			total += c.Count
			vendor := extractor.GetVender(c.ID)
			if vendorTotals[vendor] == 0 {
				vendorOrder = append(vendorOrder, vendor)
			}
			vendorTotals[vendor] += c.Count
		}
		VendorCounts := make([]extractor.VendorCount, 0, len(vendorOrder))
		for _, v := range vendorOrder {
			VendorCounts = append(VendorCounts, extractor.VendorCount{Vendor: v, Count: vendorTotals[v]})
		}
		results = append(results, OLTInventory{
			Host:         olt.Host,
			Site:         olt.Site,
			Device:       olt.Device,
			Counts:       counts,
			Vendorcounts: VendorCounts,
			Total:        total,
		})
	}
	return results
}
