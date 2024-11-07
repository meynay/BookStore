package functions

import (
	"log"

	"golang.org/x/crypto/bcrypt"
)

func CompareHashAndPassword(hashed, pass string) error {

	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(pass))
}

func Exists(value int, arr []int) bool {
	for _, val := range arr {
		if value == val {
			return true
		}
	}
	return false
}

func HashPassword(password string) (string, error) {
	// Generate a hashed password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	log.Println(string(hash))
	return string(hash), nil
}

func CheckCompatibility(arr1, arr2 []int) bool {
	for _, val := range arr1 {
		if !Exists(val, arr2) {
			return false
		}
	}
	return true
}
