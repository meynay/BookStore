package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meynay/BookStore/functions"
	"github.com/meynay/BookStore/models"
)

type App struct {
	DB *sql.DB
}

func (app *App) GetBooks(c *gin.Context) {
	books := []models.LowBook{}
	gotbooks, _ := app.DB.Query("SELECT book_id, title, image_url, price FROM book ORDER BY RANDOM() LIMIT 500")
	for gotbooks.Next() {
		var book_id int
		var title string
		var image_url string
		var price int
		if err := gotbooks.Scan(&book_id, &title, &image_url, &price); err != nil {
			log.Println("Couldn't bind book")
		} else {
			book := models.LowBook{
				Title:    title,
				Id:       book_id,
				ImageUrl: image_url,
				Price:    price,
			}
			books = append(books, book)
		}
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) GetBooksByGenre(c *gin.Context) {
	genre := c.Param("genre")
	var bids []int
	gotbookids, err := app.DB.Query("SELECT book_id FROM book_genre WHERE genre=$1", genre)
	if err != nil {
		c.String(http.StatusBadRequest, "Bad request")
		return
	}
	for gotbookids.Next() {
		var id int
		if err := gotbookids.Scan(&id); err != nil {
			log.Fatal(err)
		}
		bids = append(bids, id)
	}
	str := fmt.Sprintf("(%d", bids[0])
	for _, bid := range bids[1:] {
		str = fmt.Sprintf("%s, %d", str, bid)
	}
	str += ")"
	res, _ := app.DB.Prepare(fmt.Sprintf("SELECT book_id, title, image_url, price FROM book WHERE book_id IN %s", str))
	gotbooks, err := res.Query()
	if err != nil {
		c.String(http.StatusNoContent, "Couldn't find books")
		return
	}
	books := []models.LowBook{}
	for gotbooks.Next() {
		var id, price int
		var title, image_url string
		if err := gotbooks.Scan(&id, &title, &image_url, &price); err == nil {
			book := models.LowBook{
				Title:    title,
				Id:       id,
				ImageUrl: image_url,
				Price:    price,
			}
			books = append(books, book)
		}
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) GetBook(c *gin.Context) {
	bid, _ := strconv.Atoi(c.Param("id"))
	gotbooks, err := app.DB.Query("SELECT * FROM book WHERE book_id = $1", bid)
	if err != nil {
		c.String(http.StatusNotFound, "Book not found!")
	}
	for gotbooks.Next() {
		var book_id int
		var title string
		var isbn string
		var image_url string
		var publication_date time.Time
		var isbn13 string
		var num_pages int
		var publisher string
		var book_format string
		var description string
		var price int
		var quantity_sale int
		var quantity_lib int
		if err := gotbooks.Scan(&book_id, &title, &isbn, &image_url, &publication_date, &isbn13, &num_pages, &publisher, &book_format, &description, &price, &quantity_sale, &quantity_lib); err != nil {
			c.String(http.StatusForbidden, "Couldn't bind")
			return
		} else {
			genres := []string{}
			genreRow, err := app.DB.Query("SELECT genre FROM book_genre WHERE book_id=$1", book_id)
			if err == nil {
				for genreRow.Next() {
					var g string
					if err := genreRow.Scan(&g); err != nil {
						log.Println("Couldn't bind author")
					} else {
						genres = append(genres, g)
					}
				}
			}
			authors := []models.AuthorR{}
			authorRow, err := app.DB.Query("SELECT role, author_id FROM book_author WHERE book_id=$1", book_id)
			if err == nil {
				for authorRow.Next() {
					var role, authorid string
					if err := authorRow.Scan(&role, &authorid); err != nil {
						log.Println("Couldn't bind author")
					} else {
						aid, _ := strconv.Atoi(authorid)
						tempi, err := app.DB.Query("SELECT name FROM authors WHERE author_id=$1", aid)
						if err == nil {
							tempi.Next()
							var name string
							if err := tempi.Scan(&name); err == nil {
								newauth := models.AuthorR{
									Author: name,
									Role:   role,
								}
								authors = append(authors, newauth)
							}
						}
					}
				}
			}
			average_rate := float32(0)
			count := 0
			rates, err := app.DB.Query("SELECT rate FROM user_rates WHERE book_id=$1", book_id)
			if err == nil {
				for rates.Next() {
					var rate int
					if err := rates.Scan(&rate); err != nil {
						log.Fatal(err)
					}
					average_rate += float32(rate)
					count++
				}
				average_rate /= float32(count)
			}
			book := models.Book{
				Title:           title,
				Id:              book_id,
				Isbn:            isbn,
				ImageUrl:        image_url,
				PublicationDate: publication_date,
				Isbn13:          isbn13,
				NumberOfPages:   num_pages,
				Publisher:       publisher,
				Format:          book_format,
				Description:     description,
				QuantityForSale: quantity_sale,
				QuantityInLib:   quantity_lib,
				Price:           price,
				Authors:         authors,
				Genres:          genres,
				AverageRate:     average_rate,
				RateCount:       count,
			}
			c.JSON(http.StatusOK, book)
		}
	}
}

func (app *App) RecommendByRecord(c *gin.Context) {
	id := c.Param("user_id")
	res, err := app.DB.Query("SELECT book_id FROM user_read WHERE userid = $1", id)
	if err != nil {
		c.String(http.StatusNotFound, "No books read by user")
		return
	}
	var bids []int
	for res.Next() {
		var bid int
		if err := res.Scan(&bid); err != nil {
			c.String(http.StatusConflict, "couldn't bind")
		}
		bids = append(bids, bid)
	}
	all := []models.FPG{}
	jsonfile, _ := os.Open("FP-Growth/rules.json")
	byteread, _ := ioutil.ReadAll(jsonfile)
	json.Unmarshal(byteread, &all)
	result := []int{}
	for i := range all {
		if functions.CheckCompatibility(bids, all[i].Base) {
			for _, number := range all[i].Res {
				if !functions.Exists(number, result) {
					result = append(result, number)
				}
			}
		}
	}
	if len(result) > 1 {
		str := fmt.Sprintf("(%d", result[0])
		for _, val := range result[1:] {
			str = fmt.Sprintf("%s, %d", str, val)
		}
		str += ")"
		books := []models.LowBook{}
		res, err := app.DB.Query(fmt.Sprintf("SELECT title, book_id, price, image_url FROM book WHERE book_id in %s", str))
		if err != nil {
			c.String(http.StatusConflict, "couldn't find books")
			return
		}
		for res.Next() {
			var book models.LowBook
			res.Scan(&book.Title, &book.Id, &book.Price, &book.ImageUrl)
			books = append(books, book)
		}
		c.JSON(http.StatusOK, books)
		return
	} else if len(result) == 1 {
		res, err := app.DB.Query("SELECT title, book_id, price, image_url FROM book WHERE book_id=$1", result[0])
		if err != nil {
			c.String(http.StatusConflict, "couldn't find books")
			return
		}
		res.Next()
		var book models.LowBook
		res.Scan(&book.Title, &book.Id, &book.Price, &book.ImageUrl)
		c.JSON(http.StatusOK, book)
		return
	}
	c.String(http.StatusNotFound, "No books found")
}

func (app *App) Login(c *gin.Context) {
	user := models.UserLogin{}
	c.BindJSON(&user)
	pass, err := functions.HashPassword(user.Password)
	if err != nil {
		c.String(http.StatusNotAcceptable, "password error")
		return
	}
	res, err := app.DB.Query("SELECT user_id, firstname, lastname, phone, email, image FROM users WHERE email=$1, password=$2", user.Email, pass)
	if err != nil {
		c.String(http.StatusNotFound, "User not found")
	}
	res.Next()
	theuser := models.User{}
	err = res.Scan(&theuser.Id, &theuser.Firstname, &theuser.Lastname, &theuser.Phone, &theuser.Email, &theuser.Image)
	if err != nil {
		c.String(http.StatusConflict, "Couldn't bind user")
		return
	}
	c.JSON(http.StatusOK, theuser)
}

func (app *App) Signup(c *gin.Context) {
	var user models.User
	c.BindJSON(&user)
	_, err := app.DB.Query("SELECT * FROM users WHERE phone=$1 OR email=$2", user.Phone, user.Email)
	if err == nil {
		c.String(http.StatusNotAcceptable, "Email or phone have already been used")
		return
	}
	user.Password, err = functions.HashPassword(user.Password)
	if err != nil {
		c.String(http.StatusConflict, "Password error")
		return
	}
	res, err := app.DB.Query("SELECT user_id FROM users ORDER BY user_id DESC LIMIT BY 1")
	if err != nil {
		user.Id = 1
	} else {
		res.Next()
		var id int
		res.Scan(&id)
		user.Id = id
	}
	app.DB.Exec("INSERT INTO users(user_id, firstname, lastname, password, phone, email, image) values ($1, $2, $3, $4, $5, $6, $7)", user.Id, user.Firstname, user.Lastname, user.Password, user.Phone, user.Email, user.Image)
	c.String(http.StatusOK, "Signup successful")
}

func (app *App) RecommendByRates(c *gin.Context) {
	c.String(http.StatusOK, "well")
}
