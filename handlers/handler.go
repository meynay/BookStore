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
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/meynay/BookStore/functions"
	"github.com/meynay/BookStore/models"
)

type App struct {
	DB *sql.DB
}

func (app *App) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenValue := c.GetHeader("Authorization")
		claims := &models.Claims{}
		tkn, err := jwt.ParseWithClaims(tokenValue, claims,
			func(token *jwt.Token) (interface{}, error) {
				return []byte(os.Getenv("JWT_SECRET")), nil
			})
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		if tkn == nil || !tkn.Valid {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		c.Next()
	}
}

func (app *App) GetBooks(c *gin.Context) {
	books := []models.LowBook{}
	gotbooks, err := app.DB.Query("SELECT book_id, title, image_url, price, avg_rate, rate_count FROM book ORDER BY RANDOM() LIMIT 500")
	if err != nil {
		c.String(http.StatusBadRequest, "couldn't get books")
		return
	}
	defer gotbooks.Close()
	for gotbooks.Next() {
		var book_id int
		var title string
		var image_url string
		var price, count int
		var rate float64
		if err := gotbooks.Scan(&book_id, &title, &image_url, &price, &rate, &count); err != nil {
			log.Println("Couldn't bind book")
		} else {
			book := models.LowBook{
				Title:    title,
				Id:       book_id,
				ImageUrl: image_url,
				Price:    price,
				Rate:     rate,
				Count:    count,
			}
			books = append(books, book)
		}
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) GetNewBooks(c *gin.Context) {
	res, err := app.DB.Query("SELECT * FROM newbook")
	if err != nil {
		c.String(http.StatusNotFound, "No new books found!")
		return
	}
	bids := []int{}
	for res.Next() {
		var bid int
		var t time.Time
		res.Scan(&bid, &t)
		if time.Since(t) > time.Duration(720)*time.Hour {
			app.DB.Exec("DELETE * FROM newbook WHERE book_id=$1", bid)
		} else {
			bids = append(bids, bid)
		}
	}
	res.Close()
	placeholders := []string{}
	for i := range bids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}
	query := fmt.Sprintf("SELECT book_id, title, image_url, price, avg_rate, rate_count FROM book WHERE book_id IN (%s)", strings.Join(placeholders, ", "))
	res, err = app.DB.Query(query, functions.ConvertToInterfaceSlice(bids)...)
	if err != nil {
		c.String(http.StatusBadRequest, "Error getting books")
		return
	}
	defer res.Close()
	books := []models.LowBook{}
	for res.Next() {
		var book models.LowBook
		res.Scan(&book.Id, &book.Title, &book.ImageUrl, &book.Price, &book.Rate, &book.Count)
		books = append(books, book)
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) GetBook(c *gin.Context) {
	bid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Error getting book id")
		return
	}
	gotbooks, err := app.DB.Query("SELECT book_id, title, isbn, image_url, publication_date, isbn13, num_pages, publisher, book_format, description, price, quantity_sale, quantity_lib, avg_rate, rate_count FROM book WHERE book_id = $1", bid)
	if err != nil || !gotbooks.Next() {
		c.String(http.StatusNotFound, "Book not found!")
		return
	}
	defer gotbooks.Close()
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
	var quantity_lib, rate_count int
	var avg_rate float64
	if err := gotbooks.Scan(&book_id, &title, &isbn, &image_url, &publication_date, &isbn13, &num_pages, &publisher, &book_format, &description, &price, &quantity_sale, &quantity_lib, &avg_rate, &rate_count); err != nil {
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
			AverageRate:     avg_rate,
			RateCount:       rate_count,
		}
		c.JSON(http.StatusOK, book)
	}
}

func (app *App) CheckIfFaved(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	var js struct {
		Id int `json:"book_id"`
	}
	c.BindJSON(&js)
	bid := js.Id
	res, err := app.DB.Query("SELECT * FROM user_fave WHERE book_id=$1 AND user_id=$2", bid, uid)
	if err != nil || !res.Next() {
		c.String(http.StatusNotFound, "Not added before")
		return
	}
	res.Close()
	c.String(http.StatusOK, "Added before")
}

func (app *App) FaveOrUnfave(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	var js struct {
		Id int `json:"book_id"`
	}
	c.BindJSON(&js)
	res, err := app.DB.Query("SELECT * FROM user_fave WHERE book_id=$1 AND user_id=$2", js.Id, uid)
	log.Println(js.Id)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured")
		return
	}
	defer res.Close()
	if !res.Next() {
		app.DB.Exec("INSERT INTO user_fave(book_id, user_id) values($1, $2)", js.Id, uid)
		c.String(http.StatusOK, "Book added to faves")
		return
	}
	app.DB.Exec("DELETE FROM user_fave WHERE book_id=$1 AND user_id=$2", js.Id, uid)
	c.String(http.StatusOK, "Book deleted from faves")
}

func (app *App) RecommendByRecord(c *gin.Context) {
	FP_GROWTH_ROUTE := os.Getenv("FP_GROWTH_ROUTE")
	id := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT book_id FROM user_read WHERE userid = $1", id)
	if err != nil || !res.Next() {
		c.String(http.StatusNotFound, "No books read by user")
		return
	}
	var bids []int
	var bid int
	if err := res.Scan(&bid); err != nil {
		c.String(http.StatusConflict, "couldn't bind")
	}
	bids = append(bids, bid)
	for res.Next() {
		var bid int
		if err := res.Scan(&bid); err != nil {
			c.String(http.StatusConflict, "couldn't bind")
		}
		bids = append(bids, bid)
	}
	res.Close()
	all := []models.FPG{}
	jsonfile, err := os.Open(FP_GROWTH_ROUTE)
	if err != nil {
		c.String(http.StatusBadRequest, "Noway")
	}
	byteread, err := ioutil.ReadAll(jsonfile)
	if err != nil {
		c.String(http.StatusBadRequest, "Noway2")
	}
	err = json.Unmarshal(byteread, &all)
	if err != nil {
		c.String(http.StatusBadRequest, "Noway3")
	}
	result := []int{}
	resMap := make(map[int]struct{})
	for i := range all {
		if functions.CheckCompatibility(bids, all[i].Base) {
			for _, number := range all[i].Res {
				if _, exists := resMap[number]; !exists {
					resMap[number] = struct{}{}
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
		res, err := app.DB.Query(fmt.Sprintf("SELECT title, book_id, price, image_url, rate_count, avg_rate FROM book WHERE book_id in %s", str))
		if err != nil {
			c.String(http.StatusConflict, "couldn't find books")
			return
		}
		for res.Next() {
			var book models.LowBook
			res.Scan(&book.Title, &book.Id, &book.Price, &book.ImageUrl, &book.Count, &book.Rate)
			books = append(books, book)
		}
		res.Close()
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
		res.Close()
		return
	}
	c.String(http.StatusNotFound, "No books found")
}

func (app *App) Login(c *gin.Context) {
	user := models.UserLogin{}
	err := c.BindJSON(&user)
	if err != nil {
		c.String(http.StatusBadRequest, "Couldn't bind json file")
		return
	}
	user.Email = strings.ToLower(user.Email)
	res, err := app.DB.Query("SELECT user_id, password FROM users WHERE email=$1", user.Email)
	if err != nil || !res.Next() {
		c.String(http.StatusNotFound, "Email not found")
		return
	}
	defer res.Close()
	var id int
	var pass string
	err = res.Scan(&id, &pass)
	log.Println(id, pass)
	if err != nil {
		c.String(http.StatusConflict, "Couldn't bind user")
		return
	}
	err = functions.CompareHashAndPassword(pass, user.Password)
	if err != nil {
		c.String(http.StatusNotAcceptable, "Wrong password")
		return
	}
	expirationTime := time.Now().Add(30 * time.Minute)
	claims := &models.Claims{
		Uid: id,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusInternalServerError,
			gin.H{"error": err.Error()})
		return
	}
	jwtOutput := models.JWTOutput{
		Token:   tokenString,
		Expires: expirationTime,
	}
	c.JSON(http.StatusOK, jwtOutput)
}

func (app *App) Signup(c *gin.Context) {
	var user models.User
	err := c.BindJSON(&user)
	if err != nil {
		c.String(http.StatusBadRequest, "Couldn't bind user")
		return
	}
	user.Email = strings.ToLower(user.Email)
	res, err := app.DB.Query("SELECT * FROM users WHERE phone=$1 OR email=$2", user.Phone, user.Email)
	if res.Next() || err != nil {
		c.String(http.StatusNotAcceptable, "Email or phone have already been used")
		return
	}
	res.Close()
	user.Password, err = functions.HashPassword(user.Password)
	if err != nil {
		c.String(http.StatusConflict, "Password error")
		return
	}
	res, _ = app.DB.Query("SELECT user_id FROM users ORDER BY user_id DESC LIMIT 1")
	defer res.Close()
	res.Next()
	var id int
	res.Scan(&id)
	user.Id = id + 1
	user.Role = false
	app.DB.Exec("INSERT INTO users(user_id, firstname, lastname, password, phone, email, image, role) values ($1, $2, $3, $4, $5, $6, $7, $8)", user.Id, user.Firstname, user.Lastname, user.Password, user.Phone, user.Email, user.Image, user.Role)
	c.String(http.StatusOK, "Signup successful")
}

func (app *App) RecommendByRates(c *gin.Context) {
	id := functions.GetUserId(c.GetHeader("Authorization"))
	req := fmt.Sprintf("http://localhost:9823/%d", id)
	res, err := http.Get(req)
	if err != nil {
		fmt.Printf("error making http request: %s\n", err)
		c.String(http.StatusBadRequest, "badone")
		return
	}
	if res.StatusCode != http.StatusOK {
		c.String(http.StatusBadRequest, "not enough books to classify")
		return
	}
	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		c.String(http.StatusBadRequest, "couldn't read json body")
		return
	}
	var bids []int
	json.Unmarshal(resBody, &bids)
	if len(bids) == 0 {
		c.String(http.StatusNotFound, "No books found")
		return
	}
	placeholders := []string{}
	for i := range bids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}
	query := fmt.Sprintf("SELECT book_id, title, image_url, price, avg_rate, rate_count FROM book WHERE book_id IN (%s)", strings.Join(placeholders, ", "))
	res2, err := app.DB.Query(query, functions.ConvertToInterfaceSlice(bids)...)
	if err != nil {
		c.String(http.StatusBadRequest, "Error getting books")
		return
	}
	defer res2.Close()
	books := []models.LowBook{}
	for res2.Next() {
		var book models.LowBook
		res2.Scan(&book.Id, &book.Title, &book.ImageUrl, &book.Price, &book.Rate, &book.Count)
		books = append(books, book)
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) AddBook(c *gin.Context) {
	id := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT role from users WHERE user_id=$1", id)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured to DB")
		return
	}
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.String(http.StatusBadRequest, "You cannot add book if you are not an admin!!")
		return
	}
	res.Close()
	var book models.Book
	err = c.BindJSON(&book)
	if err != nil {
		c.String(http.StatusBadRequest, "Error binding json")
		return
	}
	res, err = app.DB.Query("SELECT book_id FROM book ORDER BY book_id DESC LIMIT 1")
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured to DB")
		return
	}
	defer res.Close()
	res.Next()
	var bid int
	res.Scan(&bid)
	bid += 1
	book.Id = bid
	_, err = app.DB.Exec("INSERT INTO book(book_id, title, isbn, image_url, publication_date, isbn13, num_pages, publisher, book_format, description, price, quantity_sale, quantity_lib, avg_rate, rate_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)", book.Id, book.Title, book.Isbn, book.ImageUrl, book.PublicationDate, book.Isbn13, book.NumberOfPages, book.Publisher, book.Format, book.Description, book.Price, book.QuantityForSale, book.QuantityInLib, book.AverageRate, book.RateCount)
	if err != nil {
		c.String(http.StatusConflict, "Couldn't record to DB")
		return
	}
	app.DB.Exec("INSERT INTO newbook(book_id, time_added) VALUES($1, $2)", bid, time.Now())
	for _, genre := range book.Genres {
		app.DB.Exec("INSERT INTO book_genre(book_id, genre) VALUES($1, $2)", bid, genre)
	}
	for _, Author := range book.Authors {
		res, err = app.DB.Query("SELECT author_id FROM authors WHERE name=$1", Author.Author)
		if err == nil {
			res.Next()
			var id int
			res.Scan(&id)
			app.DB.Exec("INSERT INTO book_author(book_id, author_id, role) VALUES($1, $2, $3)", bid, id, Author.Role)
		} else {
			res, _ = app.DB.Query("SELECT author_id FROM authors ORDER BY author_id DESC LIMIT 1")
			res.Next()
			var aid int
			res.Scan(&aid)
			aid += 1
			app.DB.Exec("INSERT INTO authors(author_id, name) VALUES($1, $2)", aid, Author.Author)
			app.DB.Exec("INSERT INTO book_author(book_id, author_id) VALUES($1, $2)", bid, aid)
		}
	}
	c.String(http.StatusOK, "book added to DB")
}

func (app *App) EditBook(c *gin.Context) {
	id := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT role from users WHERE user_id=$1", id)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured to DB")
		return
	}
	defer res.Close()
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.String(http.StatusBadRequest, "You cannot add book if you are not an admin!!")
		return
	}
	var book models.Book
	if err := c.BindJSON(&book); err != nil {
		c.String(http.StatusBadRequest, "Cannot bind json")
		return
	}
	app.DB.Exec("UPDATE book SET title=$1 and isbn=$2 and image_url=$3 and publication_date=$4 and isbn13=$5 and num_pages=$6 and publisher=$7 and book_format=&8 and description=$9 and price=$10 and quantity_sale=$11 and quantity_lib=$12", book.Title, book.Isbn, book.ImageUrl, book.PublicationDate, book.Isbn13, book.NumberOfPages, book.Publisher, book.Format, book.Description, book.Price, book.QuantityForSale, book.QuantityInLib)
	c.String(http.StatusOK, "Book updated")
}

func (app *App) GetUserProfile(c *gin.Context) {
	id := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT firstname, lastname, image FROM users WHERE user_id=$1", id)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured to DB")
		return
	}
	defer res.Close()
	res.Next()
	var fname, lname, image string
	res.Scan(&fname, &lname, &image)
	c.JSON(http.StatusOK, gin.H{
		"firstname": fname,
		"lastname":  lname,
		"image":     image,
	})
}

func (app *App) FilterBooks(c *gin.Context) {
	filters := models.Filter{}
	if err := c.ShouldBindBodyWithJSON(&filters); err != nil {
		c.String(http.StatusBadRequest, "Couldn't bind json")
		return
	}
	books := []models.Book{}
	if len(filters.Genres) > 0 {
		res, err := app.DB.Query("SELECT book_id FROM book_genre WHERE genre in $1", filters.Genres)
		if err != nil {
			c.String(http.StatusBadRequest, "Error occured to DB")
			return
		}
		book_ids := []int{}
		for res.Next() {
			var id int
			res.Scan(&id)
			book_ids = append(book_ids, id)
		}
		res.Close()
		res, err = app.DB.Query("SELECT book_id, title, image_url, publication_date, num_pages, avg_rate, rate_count, publisher FROM book WHERE book_id in $1", book_ids)
		if err != nil {
			c.String(http.StatusBadRequest, "Error occured to DB")
			return
		}
		for res.Next() {
			var b models.Book
			res.Scan(&b.Id, &b.Title, &b.ImageUrl, &b.PublicationDate, &b.NumberOfPages, &b.AverageRate, &b.RateCount, &b.Publisher)
			books = append(books, b)
		}
		res.Close()
	} else {
		res, err := app.DB.Query("SELECT book_id, title, image_url, publication_date, num_pages, avg_rate, rate_count, publisher FROM book")
		if err != nil {
			c.String(http.StatusBadRequest, "Error occured to DB")
			return
		}
		for res.Next() {
			var b models.Book
			res.Scan(&b.Id, &b.Title, &b.ImageUrl, &b.PublicationDate, &b.NumberOfPages, &b.AverageRate, &b.RateCount, &b.Publisher)
			books = append(books, b)
		}
		res.Close()
	}
	if filters.Search != "" {
		filters.Search = strings.ToLower(filters.Search)
		for id, book := range books {
			if !strings.Contains(strings.ToLower(book.Title), filters.Search) && !strings.Contains(strings.ToLower(book.Publisher), filters.Search) {
				books = append(books[:id], books[id+1:]...)
			}
		}
	}
	for id, book := range books {
		if !(book.NumberOfPages > filters.MinPages && book.NumberOfPages < filters.MaxPages) || !(book.PublicationDate.After(filters.StartDate) && book.PublicationDate.Before(filters.EndDate)) {
			books = append(books[:id], books[id+1:]...)
		}
	}
	if len(books) == 0 {
		c.String(http.StatusBadRequest, "No books found for given filters")
		return
	}
	returning := []models.LowBook{}
	for _, book := range books {
		returning = append(returning, models.LowBook{
			Title:    book.Title,
			ImageUrl: book.ImageUrl,
			Rate:     book.AverageRate,
			Count:    book.RateCount,
			Id:       book.Id,
			Price:    book.Price,
		})
	}
	c.JSON(http.StatusOK, returning)
}

func (app *App) RateBook(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	var rate models.Rate
	err := c.BindJSON(&rate)
	if err != nil {
		c.String(http.StatusBadRequest, "Error binding json")
		return
	}
	var rating int
	var b bool = false
	res, err := app.DB.Query("SELECT rating FROM user_rating WHERE user_id=$1 AND book_id=$2", uid, rate.Bid)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured to DB")
		return
	}
	if res.Next() {
		res.Scan(&rating)
		b = true
	}
	date_added := time.Now()
	_, err = app.DB.Exec("INSERT INTO user_rating(user_id, book_id, rating, review, date_added) values($1, $2, $3, $4, $5)", uid, rate.Bid, rate.Rating, rate.Review, date_added)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured")
		return
	}
	res.Close()
	res, err = app.DB.Query("SELECT avg_rate, rate_count FROM book WHERE book_id=$1", rate.Bid)
	if err != nil {
		c.String(http.StatusBadRequest, "Err occured")
		return
	}
	defer res.Close()
	res.Next()
	var avg float64
	var count int
	res.Scan(&avg, &count)
	if b {
		avg = avg * float64(count)
		avg = avg - float64(rating) + float64(rate.Rating)
		avg /= float64(count)
	} else {
		avg = avg * float64(count)
		avg += +float64(rate.Rating)
		count++
		avg /= float64(count)
	}
	app.DB.Exec("UPDATE book SET avg_rate=$1, rate_count=$2 WHERE book_id=$3", avg, count, rate.Bid)
	c.String(http.StatusOK, "Rate added")
}

func (app *App) CommentOnBook(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	var rate models.Rate
	err := c.BindJSON(&rate)
	if err != nil {
		c.String(http.StatusBadRequest, "Error binding json")
		return
	}
	app.DB.Exec("INSERT INTO comment(book_id, user_id, review, date_added) values($1, $2, $3, $4)", rate.Bid, uid, rate.Review, time.Now())
	c.String(http.StatusOK, "Comment added")
}

func (app *App) GetComments(c *gin.Context) {
	book_id, _ := strconv.Atoi(c.Param("book_id"))
	res, err := app.DB.Query("SELECT (firstname || ' ' || lastname) as name, review FROM comment INNER JOIN users ON comment.user_id=users.user_id WHERE comment.book_id=$1", book_id)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured to DB")
		return
	}
	defer res.Close()
	comments := []models.UserComment{}
	for res.Next() {
		comment := models.UserComment{}
		if err := res.Scan(&comment.Name, &comment.Comment); err != nil {
			c.String(http.StatusInternalServerError, "Couldn't find comments on this book")
			return
		}
		comments = append(comments, comment)
	}
	if len(comments) == 0 {
		c.String(http.StatusNotFound, "No comments found")
	}
	c.JSON(http.StatusOK, comments)
}

func (app *App) GetRates(c *gin.Context) {
	book_id, _ := strconv.Atoi(c.Param("book_id"))
	res, err := app.DB.Query("SELECT (firstname || ' ' || lastname) as name, review, rating FROM user_rating INNER JOIN users ON user_rating.user_id=users.user_id WHERE user_rating.book_id=$1", book_id)
	if err != nil {
		c.String(http.StatusBadRequest, "Error occured")
		return
	}
	defer res.Close()
	comments := []models.UserComment{}
	for res.Next() {
		comment := models.UserComment{}
		if err := res.Scan(&comment.Name, &comment.Comment, &comment.Rate); err != nil {
			c.String(http.StatusInternalServerError, "Couldn't find comments on this book")
			return
		}
		comments = append(comments, comment)
	}
	if len(comments) == 0 {
		c.String(http.StatusNotFound, "No comments found")
	}
	c.JSON(http.StatusOK, comments)
}
