package utils

import (
	"encoding/json"
	"log"
	"os"
)

func Expect(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v\n", msg, err)
	}
}

func LoadJSON(file string, v interface{}) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, v)
	return err
}
