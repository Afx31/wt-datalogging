package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"go.einride.tech/can/pkg/socketcan"
)

func main() {
	// Config, move to a config file later
	// cansend vcan0 069#11223344
	configCanDevice := "vcan0"
	configStopDataloggingId := uint32(105) //hex = 69
	// hertz options [200 = 5hz | 100 = 10hz | 50 = 20hz]
	configHertz := 100

	
	duration := time.Duration(configHertz) * time.Millisecond
	ticker := time.NewTicker(duration) // Create a ticker that ticks every 100 milliseconds
	quit := make(chan struct{})// Channel to signal when to stop the ticker

	fmt.Println("--- Datalogging initialising... ---")

	// -------------------- Create CSV file and write headers to it --------------------
	file, err := os.Create("data.csv")
	if err != nil {
		log.Fatalf("Error creating file: %v", err)
	}
	defer file.Close()

	// Create a CSV writer that writes directly to the file
	w := csv.NewWriter(file)
	defer w.Flush()

	headers := "id,dlc,b0,b1,b2,b3,b4,b5,b6,b7"
	csvHeaders := strings.Split(headers, ",")

	if err := w.Write(csvHeaders); err != nil {
		log.Fatalln("Error writing headers to CSV", err)
	}


	// -------------------- Read CAN and write to file --------------------
	conn, _ := socketcan.DialContext(context.Background(), "can", configCanDevice)
	defer conn.Close()
	recv := socketcan.NewReceiver(conn)

	for recv.Receive() {
		frame := recv.Frame()
		
		// Button input from user to stop the datalogging
		if frame.ID == configStopDataloggingId {
			return
		}

		// if err := frame.Validate(); err != nil {
		// 	fmt.Println("Error validating frame:", err)
		// }

		var hexData []string
		for i := 0; i < int(frame.Length); i++ {
			hexData = append(hexData, fmt.Sprintf("%02X", frame.Data[i]))
		}
		
		csvFrame := append([]string{
			strconv.FormatUint(uint64(frame.ID), 10),
			strconv.FormatUint(uint64(frame.Length), 10),
		}, hexData...)

		if err := w.Write(csvFrame); err != nil {
			log.Fatalln("Error writing headers to CSV", err)
		}
	}

	// Flush any buffered data to ensure all data is written to the file
	w.Flush()

	if err := w.Error(); err != nil {
		log.Fatal(err)
	}

	log.Println("CSV file 'data.csv' has been created successfully.")
}