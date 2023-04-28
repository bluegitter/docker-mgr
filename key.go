package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func main1() {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		fmt.Println("Error generating random key:", err)
		return
	}
	keyBase64 := base64.StdEncoding.EncodeToString(key)
	fmt.Println("Your signing key:", keyBase64)
}
