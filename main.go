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

// DefaultMicColors is used if micColors is not set in .env
const DefaultMicColors = "Red Blue Purple Yellow Green Pastor1 Pastor2 Orange"

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

	micColors := environment["micColors"]
	if micColors == "" {
		micColors = DefaultMicColors
	}
	micsList := strings.Split(micColors, " ")

	mics := Mics{Mic: make([]MicStatus, len(micsList))}
	for i := range mics.Mic {
		macroName := fmt.Sprintf("%s_On", micsList[i])
		mics.Mic[i] = MicStatus{Name: micsList[i], Active: true, MacroName: macroName}
	}

	defaultSoundSource := environment["default_sound_source"]

	startServer(client, mics, proCfg, mixingCfg, defaultSoundSource)
}
