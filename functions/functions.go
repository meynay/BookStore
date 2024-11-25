package functions

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-redis/redis/v8"
	"github.com/meynay/BookStore/models"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/gomail.v2"
)

var Redis_Host = os.Getenv("REDIS_HOST")
var Redis_Port = os.Getenv("REDIS_PORT")

var ctx = context.Background()
var redisClient = redis.NewClient(&redis.Options{
	Addr: Redis_Host + Redis_Port,
})

func BlacklistToken(token string, expiry time.Duration) error {
	return redisClient.Set(ctx, token, "blacklisted", expiry).Err()
}

func IsTokenBlacklisted(token string) bool {
	result, err := redisClient.Get(ctx, token).Result()
	return err == nil && result == "blacklisted"
}

func ConvertToInterfaceSlice(bids []int) []interface{} {
	result := make([]interface{}, len(bids))
	for i, v := range bids {
		result[i] = v
	}
	return result
}

func ConvertToInterfaceSlices(str []string) []interface{} {
	result := make([]interface{}, len(str))
	for i, v := range str {
		result[i] = v
	}
	return result
}

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

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	log.Println(string(hash))
	return string(hash), nil
}

func Exists(value int, arr []int) bool {
	for _, val := range arr {
		if value == val {
			return true
		}
	}
	return false
}

func CheckCompatibility(arr1, arr2 []int) bool {
	x := len(arr1)
	if len(arr2) < x {
		x = len(arr2)
	}
	x = x / 2
	xount := 0
	for _, val := range arr1 {
		if Exists(val, arr2) {
			xount++
		}
	}
	if xount >= x && xount >= 1 {
		return true
	}
	return false
}

func SendEmail(to, subject, body string, config models.EmailConfig) error {
	m := gomail.NewMessage()
	m.SetHeader("From", config.SenderEmail)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)
	log.Println(fmt.Sprintf("Sending mail to %s From %s Subject is %s", to, config.SenderEmail, subject))
	d := gomail.NewDialer(config.SMTPHost, config.SMTPPort, config.Username, config.Password)
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func SendResetPassEmail(email, token string, config models.EmailConfig) error {
	resetLink := fmt.Sprintf("https://bikaransystem.work.gd/reset-password?token=%s", token)
	subject := "بازیابی رمز عبور"
	body := fmt.Sprintf(`
        <p>ما یک درخواست برای بازیابی رمز عبور دریافت کردیم</p>
        <p>برای بازیابی رمز عبور خود برروی لینک زیر کلیک کنید</p>
        <a href="%s">بازیابی رمز عبور</a>
        <p>اگر این درخواست توسط شما ثبت نشده است، به این ایمیل توجه نکنید!</p>
    `, resetLink)
	return SendEmail(email, subject, body, config)
}

func GetBorrowedBooks(result *sql.Rows) []models.BorrowedBook {
	books := []models.BorrowedBook{}
	for result.Next() {
		var book models.BorrowedBook
		result.Scan(&book.Book.Id, &book.Book.Title, &book.Book.ImageUrl, &book.Book.Rate, &book.Book.Count, &book.BorrowTime, &book.Returned)
		books = append(books, book)
	}
	return books
}
