package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/xuri/excelize/v2"
)

type FlatRow struct {
	Host   string  `json:"host"`
	OntIdx string  `json:"ont_idx"`
	Rx     float64 `json:"rx"`
	Desc1  string  `json:"desc1"`
	Desc2  string  `json:"desc2"`
}

func SaveExcel(folder, filename string, v any) (string, error) {
	if folder == "" {
		folder = "json"
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", folder, err)
	}
	if filepath.Ext(filename) != ".xlsx" {
		filename += ".xlsx"
	}
	path := filepath.Join(folder, filename)

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice || rv.Len() == 0 {
		return "", fmt.Errorf("expected a non-empty slice")
	}

	// Write headers from struct json tags
	elemType := rv.Index(0).Type()
	for col := 0; col < elemType.NumField(); col++ {
		field := elemType.Field(col)
		header := field.Tag.Get("json")
		if header == "" {
			header = field.Name
		}
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheet, cell, header)
	}

	// Write rows
	for row := 0; row < rv.Len(); row++ {
		elem := rv.Index(row)
		for col := 0; col < elem.NumField(); col++ {
			cell, _ := excelize.CoordinatesToCellName(col+1, row+2)
			f.SetCellValue(sheet, cell, elem.Field(col).Interface())
		}
	}

	if err := f.SaveAs(path); err != nil {
		return "", fmt.Errorf("save excel: %w", err)
	}
	return path, nil
}