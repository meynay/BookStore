package models

import (
	"time"

	"github.com/dgrijalva/jwt-go"
)

type Claims struct {
	Uid int `json:"user"`
	jwt.StandardClaims
}

type JWTOutput struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

type AuthorR struct {
	Author string `json:"author"`
	Role   string `json:"role"`
}

type LowBook struct {
	Title    string  `json:"title"`
	Id       int     `json:"id"`
	Price    int     `json:"price"`
	ImageUrl string  `json:"image_url"`
	Rate     float64 `json:"rate"`
	Count    int     `json:"rates_count"`
}

type UserLogin struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type User struct {
	Id        int    `json:"user_id"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Image     string `json:"image"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Role      bool   `json:"role"`
}

type Book struct {
	Title           string    `json:"title"`
	Id              int       `json:"id"`
	Isbn            string    `json:"isbn"`
	ImageUrl        string    `json:"imageurl"`
	PublicationDate time.Time `json:"publicationdate"`
	Isbn13          string    `json:"isbn13"`
	NumberOfPages   int       `json:"numberofpages"`
	Publisher       string    `json:"publisher"`
	Format          string    `json:"format"`
	Description     string    `json:"description"`
	QuantityForSale int       `json:"qs"`
	QuantityInLib   int       `json:"ql"`
	Price           int       `json:"price"`
	Genres          []string  `json:"genres"`
	Authors         []AuthorR `json:"authors"`
	AverageRate     float32   `json:"average_rating"`
	RateCount       int       `json:"rate_count"`
}

type FPG struct {
	Base []int `json:"base"`
	Res  []int `json:"result"`
}
