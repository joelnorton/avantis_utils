package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const MicColors string = "Red Blue Purple Yellow Red Pastor1 Pastor2 Orange"

func main() {

	environment, err := godotenv.Read(".env")
	if err != nil {
		log.Fatal(err)
	}

	proPort, err := strconv.Atoi(environment["proPort"])
	mixingPort, err := strconv.Atoi(environment["mixingPort"])
	proCfg := APIConfig{Server: environment["proServer"], Port: proPort}
	mixingCfg := APIConfig{Server: environment["mixingServer"], Port: mixingPort, Location: environment["mixingLocation"]}

	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	micsList := strings.Split(MicColors, " ")

	mics := Mics{Mic: make([]MicStatus, len(micsList))}
	for i := range mics.Mic {
		macroName := fmt.Sprintf("%s_On", micsList[i])
		mics.Mic[i] = MicStatus{Name: micsList[i], Active: true, MacroName: macroName}
	}
	startServer(client, mics, proCfg, mixingCfg)
}
