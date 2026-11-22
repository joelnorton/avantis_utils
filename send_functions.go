package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
)

func sendProMacro(client *http.Client, cfg APIConfig, macroName string) {

	//http://192.168.1.140:63234/v1/macro/23525
	url := fmt.Sprintf("http://%s:%d/v1/macro/%s/trigger", cfg.Server, cfg.Port, macroName)
	fmt.Println(url)
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func sendMixingRate(client *http.Client, cfg APIConfig, location string, value int) error {

	//http://localhost:8080/console/data/set/fx.rack.0.tl/val
	url := fmt.Sprintf("http://%s:%d/console/data/set/%s/val", cfg.Server, cfg.Port, location)
	// {"fade": 0, "value": 800.0}
	jsonData := []byte(`{"fade": 0, "value": ` + strconv.Itoa(value) + `}`)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("posting mixing rate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("mixing server returned %s", resp.Status)
	}
	return nil
}
