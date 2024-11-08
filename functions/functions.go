package functions

import (
	"log"
	"os"

	"github.com/dgrijalva/jwt-go"
	"github.com/meynay/BookStore/models"
	"golang.org/x/crypto/bcrypt"
)

func CompareHashAndPassword(hashed, pass string) error {

	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(pass))
}

func GetUserId(token string) int {
	claims := &models.Claims{}
	jwt.ParseWithClaims(token, claims,
		func(token *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
	return claims.Uid
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
