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
	"github.com/lib/pq"
	"github.com/meynay/BookStore/functions"
	"github.com/meynay/BookStore/models"
)

type App struct {
	DB         *sql.DB
	Email      models.EmailConfig
	RateLimit  models.RateLimiter
	ResetToken map[string]string
	GetSignal  map[int]chan (bool)
}

const RATELIMIT = 200
const DURATION = time.Minute

// middlewares
func (app *App) ApiKeyCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		api_key := c.GetHeader("x-api-key")
		API_KEY := os.Getenv("API_KEY")
		if api_key == API_KEY {
			c.Next()
		} else {
			c.AbortWithStatus(http.StatusForbidden)
		}
	}
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
		if tkn == nil || !tkn.Valid || functions.IsTokenBlacklisted(tokenValue) {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		c.Next()
	}
}

func (app *App) DDOSPrevent() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		app.RateLimit.Mutex.Lock()
		defer app.RateLimit.Mutex.Unlock()
		arr, exists := app.RateLimit.Visitors[ip]
		if exists {
			if len(arr) >= RATELIMIT {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"message": "send less requests"})
				return
			}
			arr = append(arr, true)
		} else {
			arr = []bool{true}
			app.RateLimit.Visitors[ip] = arr
		}
		app.RateLimit.Visitors[ip] = arr
		go func() {
			time.Sleep(DURATION)
			vis := app.RateLimit.Visitors[ip]
			vis = vis[:len(vis)-1]
			app.RateLimit.Visitors[ip] = vis
		}()
		c.Next()
	}
}

// get books
func (app *App) GetBooks(c *gin.Context) {
	books := []models.LowBook{}
	gotbooks, err := app.DB.Query("SELECT book_id, title, image_url, price, avg_rate, rate_count FROM book ORDER BY RANDOM() LIMIT 1000")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.AbortWithStatus(http.StatusNotFound)
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
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	gotbooks, err := app.DB.Query("SELECT book_id, title, isbn, image_url, publication_date, isbn13, num_pages, publisher, book_format, description, price, quantity_sale, quantity_lib, avg_rate, rate_count FROM book WHERE book_id = $1", bid)
	if err != nil || !gotbooks.Next() {
		c.AbortWithStatus(http.StatusNotFound)
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
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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
	bid, err := strconv.Atoi(c.Param("book_id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res, err := app.DB.Query("SELECT * FROM user_fave WHERE book_id=$1 AND user_id=$2", bid, uid)
	if err != nil || !res.Next() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	res.Close()
	c.String(http.StatusOK, "Added before")
}

func (app *App) FilterBooks(c *gin.Context) {
	var filters models.Filter
	if err := c.BindJSON(&filters); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	startdate := time.Date(filters.StartDate, 1, 1, 0, 0, 0, 0, time.UTC)
	enddate := time.Date(filters.EndDate, 12, 31, 23, 59, 59, 0, time.UTC)
	var queryBuilder strings.Builder
	var args []interface{}
	queryBuilder.WriteString("SELECT book_id, title, image_url, price, avg_rate, rate_count FROM book WHERE")
	queryBuilder.WriteString("(LOWER(title) LIKE $1 OR LOWER(publisher) LIKE $1) AND ")
	args = append(args, fmt.Sprintf("%%%s%%", strings.ToLower(filters.Search)))
	queryBuilder.WriteString("num_pages BETWEEN $2 AND $3 AND publication_date BETWEEN $4 AND $5 ")
	args = append(args, filters.MinPages, filters.MaxPages, startdate, enddate)
	if len(filters.Genres) > 0 {
		queryBuilder.WriteString("AND book_id IN (SELECT book_id FROM book_genre WHERE genre = ANY($6)) ")
		args = append(args, pq.Array(filters.Genres))
	}
	queryBuilder.WriteString("ORDER BY RANDOM() LIMIT 1000")
	query := queryBuilder.String()
	res, err := app.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	var books []models.LowBook
	for res.Next() {
		var b models.LowBook
		if err := res.Scan(&b.Id, &b.Title, &b.ImageUrl, &b.Price, &b.Rate, &b.Count); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		books = append(books, b)
	}
	if len(books) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No books found"})
		return
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) FaveOrUnfave(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	var js struct {
		Id int `json:"book_id"`
	}
	err := c.BindJSON(&js)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res, err := app.DB.Query("SELECT * FROM user_fave WHERE book_id=$1 AND user_id=$2", js.Id, uid)
	log.Println(js.Id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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

func (app *App) RateBook(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	var rate models.Rate
	err := c.BindJSON(&rate)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var rating int
	var b bool = false
	res, err := app.DB.Query("SELECT rating FROM user_rating WHERE user_id=$1 AND book_id=$2", uid, rate.Bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	if res.Next() {
		res.Scan(&rating)
		b = true
	}
	date_added := time.Now()
	_, err = app.DB.Exec("INSERT INTO user_rating(user_id, book_id, rating, review, date_added) values($1, $2, $3, $4, $5)", uid, rate.Bid, rate.Rating, rate.Review, date_added)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res.Close()
	res, err = app.DB.Query("SELECT avg_rate, rate_count FROM book WHERE book_id=$1", rate.Bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, err = app.DB.Exec("INSERT INTO comment(book_id, user_id, review, date_added) values($1, $2, $3, $4)", rate.Bid, uid, rate.Review, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	c.String(http.StatusOK, "Comment added")
}

func (app *App) GetComments(c *gin.Context) {
	book_id, _ := strconv.Atoi(c.Param("book_id"))
	res, err := app.DB.Query("SELECT (firstname || ' ' || lastname) as name, review FROM comment INNER JOIN users ON comment.user_id=users.user_id WHERE comment.book_id=$1", book_id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	defer res.Close()
	comments := []models.UserComment{}
	for res.Next() {
		comment := models.UserComment{}
		if err := res.Scan(&comment.Name, &comment.Comment); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer res.Close()
	comments := []models.UserComment{}
	for res.Next() {
		comment := models.UserComment{}
		if err := res.Scan(&comment.Name, &comment.Comment, &comment.Rate); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
			return
		}
		comments = append(comments, comment)
	}
	if len(comments) == 0 {
		c.String(http.StatusNotFound, "No comments found")
	}
	c.JSON(http.StatusOK, comments)
}

func (app *App) GetFavedBooks(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT book_id FROM user_fave WHERE user_id=$1", uid)
	if err != nil || !res.Next() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	bids := []int{}
	var bid int
	res.Scan(&bid)
	bids = append(bids, bid)
	for res.Next() {
		var bid int
		res.Scan(&bid)
		bids = append(bids, bid)
	}
	placeholders := []string{}
	res.Close()
	for i := range bids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}
	query := fmt.Sprintf("SELECT book_id, title, image_url, price, avg_rate, rate_count FROM book WHERE book_id IN (%s)", strings.Join(placeholders, ", "))
	res, err = app.DB.Query(query, functions.ConvertToInterfaceSlice(bids)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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

// user section
func (app *App) Login(c *gin.Context) {
	user := models.UserLogin{}
	err := c.BindJSON(&user)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	user.Email = strings.ToLower(user.Email)
	res, err := app.DB.Query("SELECT user_id, password FROM users WHERE email=$1", user.Email)
	if err != nil || !res.Next() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	defer res.Close()
	var id int
	var pass string
	err = res.Scan(&id, &pass)
	log.Println(id, pass)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	err = functions.CompareHashAndPassword(pass, user.Password)
	if err != nil {
		c.AbortWithStatus(http.StatusNotAcceptable)
		return
	}
	expirationTime := time.Now().Add(60 * time.Minute)
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

func (app *App) Logout(c *gin.Context) {
	token := c.GetHeader("Authorization")
	err := functions.BlacklistToken(token, time.Hour*24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (app *App) Signup(c *gin.Context) {
	var user models.User
	err := c.BindJSON(&user)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	user.Email = strings.ToLower(user.Email)
	res, err := app.DB.Query("SELECT * FROM users WHERE email=$1", user.Email)
	if res.Next() || err != nil {
		c.JSON(http.StatusNotAcceptable, gin.H{"message": "email alerady exists"})
		return
	}
	res.Close()
	user.Password, err = functions.HashPassword(user.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res, _ = app.DB.Query("SELECT user_id FROM users ORDER BY user_id DESC LIMIT 1")
	defer res.Close()
	res.Next()
	var id int
	res.Scan(&id)
	user.Id = id + 1
	user.Role = false
	user.Image = "tempo"
	app.DB.Exec("INSERT INTO users(user_id, firstname, lastname, password, email, image, role) values ($1, $2, $3, $4, $5, $6, $7)", user.Id, user.Firstname, user.Lastname, user.Password, user.Email, user.Image, user.Role)
	subject := "Bookstore sign up"
	body := fmt.Sprintf(`
		<h1>Welcome %s %s<h1>
        <p>We're glad that you decided to use our service. Hope you can find what you seek in our web app</p>
    `, user.Firstname, user.Lastname)
	functions.SendEmail(user.Email, subject, body, app.Email)
	c.String(http.StatusOK, "Signup successful")
}

func (app *App) GetUserProfile(c *gin.Context) {
	id := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT firstname, lastname, image FROM users WHERE user_id=$1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	defer res.Close()
	res.Next()
	var fname, lname, image string
	res.Scan(&fname, &lname, &image)
	if image == "tempo" {
		image = ""
	}
	c.JSON(http.StatusOK, gin.H{
		"firstname": fname,
		"lastname":  lname,
		"image":     image,
	})
}

func (apap *App) GetProfPic(c *gin.Context) {
	fname := c.Param("image")
	fdir := os.Getenv("FILE_DIR")
	c.File(fdir + "/" + fname)
}

func (app *App) GetUserInfo(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT firstname, lastname, email, image, role FROM users WHERE user_id=$1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	defer res.Close()
	res.Next()
	var user models.User
	if err = res.Scan(&user.Firstname, &user.Lastname, &user.Email, &user.Image, &user.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (app *App) UploadImage(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"Error": "Error occured during getting file"})
		return
	}
	fileDir := os.Getenv("FILE_DIR")
	res, err := app.DB.Query("SELECT image from users WHERE user_id=$1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	var img string
	defer res.Close()
	res.Scan(&img)
	if img != "tempo" {
		os.Remove(fmt.Sprintf("%s/%s", fileDir, img))
	}
	img = fmt.Sprintf("%d_%s", uid, file.Filename)
	err = c.SaveUploadedFile(file, fmt.Sprintf("%s/%s", fileDir, img))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	_, err = app.DB.Exec("UPDATE users SET image=$1 WHERE user_id=$2", img, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	c.String(http.StatusOK, "Image added successfully")
}

func (app *App) ResetPasswordMail(c *gin.Context) {
	var request struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"Error": "Wrong JSON format"})
		return
	}
	res, err := app.DB.Query("SELECT * FROM users WHERE email=$1", request.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	defer res.Close()
	if !res.Next() {
		c.JSON(http.StatusNotFound, gin.H{"Error": "No user found with given Email"})
		return
	}
	token, err := functions.GenerateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	app.ResetToken[token] = request.Email
	go func() {
		time.Sleep(15 * time.Minute)
		delete(app.ResetToken, token)
	}()
	if err = functions.SendResetPassEmail(request.Email, token, app.Email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"Message": "Reset Password Email sent"})
}

func (app *App) ResetPassword(c *gin.Context) {
	token := c.Param("token")
	email, ok := app.ResetToken[token]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"Error": "Invalid token"})
		return
	}
	var pass struct {
		Pass string `json:"password"`
	}
	err := c.ShouldBind(&pass)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"Error": "Bad JSON format"})
		return
	}
	password, err := functions.HashPassword(pass.Pass)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	_, err = app.DB.Exec("UPDATE users SET password=$1 WHERE email=$2", password, email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"Message": "Password changed successfully"})
}

// recommenders
func (app *App) RecommendByRates(c *gin.Context) {
	id := functions.GetUserId(c.GetHeader("Authorization"))
	req := fmt.Sprintf("http://localhost:9823/%d", id)
	res, err := http.Get(req)
	if err != nil {
		fmt.Printf("error making http request: %s\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if res.StatusCode != http.StatusOK {
		c.JSON(res.StatusCode, gin.H{"message": res.Body})
		return
	}
	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	var bids []int
	json.Unmarshal(resBody, &bids)
	if len(bids) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "no books found for user"})
		return
	}
	placeholders := []string{}
	for i := range bids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}
	query := fmt.Sprintf("SELECT book_id, title, image_url, price, avg_rate, rate_count FROM book WHERE book_id IN (%s)", strings.Join(placeholders, ", "))
	res2, err := app.DB.Query(query, functions.ConvertToInterfaceSlice(bids)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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

func (app *App) RecommendByRecord(c *gin.Context) {
	FP_GROWTH_ROUTE := os.Getenv("FP_GROWTH_ROUTE")
	id := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT book_id FROM user_read WHERE userid = $1", id)
	if err != nil || !res.Next() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var bids []int
	var bid int
	if err := res.Scan(&bid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
	}
	bids = append(bids, bid)
	for res.Next() {
		var bid int
		if err := res.Scan(&bid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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
	for i := 0; i < len(result); i++ {
		if functions.Exists(result[i], bids) {
			result = append(result[:i], result[i+1:]...)
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
			c.AbortWithStatus(http.StatusNotFound)
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
			c.String(http.StatusNotFound, "couldn't find books")
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

// book changes
func (app *App) AddBook(c *gin.Context) {
	id := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT role from users WHERE user_id=$1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.AbortWithStatus(http.StatusNotAcceptable)
		return
	}
	res.Close()
	var book models.Book
	err = c.BindJSON(&book)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res, err = app.DB.Query("SELECT book_id FROM book ORDER BY book_id DESC LIMIT 1")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	defer res.Close()
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.AbortWithStatus(http.StatusNotAcceptable)
		return
	}
	var book models.Book
	if err := c.BindJSON(&book); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	app.DB.Exec("UPDATE book SET title=$1 and isbn=$2 and image_url=$3 and publication_date=$4 and isbn13=$5 and num_pages=$6 and publisher=$7 and book_format=&8 and description=$9 and price=$10 and quantity_sale=$11 and quantity_lib=$12", book.Title, book.Isbn, book.ImageUrl, book.PublicationDate, book.Isbn13, book.NumberOfPages, book.Publisher, book.Format, book.Description, book.Price, book.QuantityForSale, book.QuantityInLib)
	c.String(http.StatusOK, "Book updated")
}

// borrow section
func (app *App) GetLibStatus(c *gin.Context) {
	bid, _ := strconv.Atoi(c.Param("bookid"))
	res, err := app.DB.Query("SELECT * FROM borrow_book WHERE book_id = $1 AND returned = 'no'", bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	if !res.Next() {
		c.JSON(http.StatusOK, gin.H{"message": "you can borrow"})
		return
	}
	c.JSON(http.StatusNotAcceptable, gin.H{"message": "can't borrow book"})
}

func (app *App) BorrowBook(c *gin.Context) {
	bid, _ := strconv.Atoi(c.Param("bookid"))
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT * FROM borrow_book WHERE user_id=$1 and resturned='no'", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if res.Next() {
		c.JSON(http.StatusNotAcceptable, gin.H{"message": "user still haven't returned last borrowed book"})
		return
	}
	_, err = app.DB.Exec("INSERT INTO borrow_book(book_id, user_id, returned, borrow_time), values($1, $2, 'no', $3)", bid, uid, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	res.Close()
	res, _ = app.DB.Query("SELECT title FROM book WHERE book_id = $1", bid)
	res.Next()
	var title, email, name string
	res.Scan(&title)
	res.Close()
	subject := "امانت کتاب"
	body := fmt.Sprintf(`<p>کتاب %s با موفقیت امانت گرفته شد</p>
	<p>برای دریافت کتاب به کتابخانه مراجعه کنید و با ارائه کارت خود کتاب را تحویل بگیرید.</p>`, title)
	res, _ = app.DB.Query("SELECT email, (firstname || ' ' || lastname) as name FROM users WHERE user_id=$1", uid)
	defer res.Close()
	res.Next()
	res.Scan(&email, &name)
	functions.SendEmail(email, subject, body, app.Email)
	app.DB.Exec("INSERT INTO user_read(book_id, userid) VALUES($1, $2)", bid, uid)
	c.JSON(http.StatusOK, gin.H{"message": "book borrowed successfully"})
	app.GetSignal[bid] = make(chan bool)
	go func() {
		select {
		case <-time.After(7 * time.Hour * 24):
			subject := "سررسید تحویل کتاب"
			body := fmt.Sprintf(`<p>سلام %s عزیز</p>
			<p>وقت تحویل کتاب %s فرارسیده. ممنون میشیم به موقع کتاب رو به کتاب خونه برگردونی!</p>`, name, title)
			functions.SendEmail(email, subject, body, app.Email)
		case <-app.GetSignal[bid]:
			return
		}
	}()
}

func (app *App) ReturnBook(c *gin.Context) {
	bid, _ := strconv.Atoi(c.Param("bookid"))
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT role from users WHERE user_id=$1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.AbortWithStatus(http.StatusNotAcceptable)
		return
	}
	res.Close()
	_, err = app.DB.Exec("UPDATE borrow_book SET returned='yes' WHERE book_id=$1 AND returned = 'no'", bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	app.GetSignal[bid] <- true
	c.JSON(http.StatusOK, gin.H{"message": "book returned successfully"})
}

func (app *App) BorrowHistory(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT book_id, title, image_url, avg_rate, rate_count, borrow_time, returned FROM borrow_book INNER JOIN book ON borrow_book.book_id = book.book_id WHERE user_id = $1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	books := functions.GetBorrowedBooks(res)
	if len(books) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "no books found for user"})
		return
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) ShowActiveBorrows(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT role from users WHERE user_id=$1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.AbortWithStatus(http.StatusNotAcceptable)
		return
	}
	res.Close()
	res, err = app.DB.Query("SELECT book_id, title, image_url, avg_rate, rate_count, borrow_time, returned FROM borrow_book INNER JOIN book ON borrow_book.book_id = book.book_id WHERE returned='no'")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	books := functions.GetBorrowedBooks(res)
	if len(books) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "no active borrows"})
		return
	}
	c.JSON(http.StatusOK, books)
}

// buy section
func (app *App) AddToCart(c *gin.Context) {
	bid, _ := strconv.Atoi(c.Param("bookid"))
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT quantity_sale FROM book WHERE book_id = $1", bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	res.Next()
	var count int
	res.Scan(&count)
	if count == 0 {
		c.JSON(http.StatusNotAcceptable, gin.H{"message": "can't add this item to your cart"})
		return
	}
	res.Close()
	res, err = app.DB.Query("SELECT invoice_id FROM invoice WHERE user_id=$1 AND status='open'", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var invoice_id int
	if !res.Next() {
		res.Close()
		res, err = app.DB.Query("SELECT invoice_id FROM invoice ORDER BY invoice_id DESC LIMIT 1")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var invoice_id int
		if !res.Next() {
			invoice_id = 1
		} else {
			res.Scan(&invoice_id)
			invoice_id++
		}
		res.Close()
		app.DB.Exec("INSERT INTO invoice(invoice_id, user_id, status, purchase_date) VALUES($1, $2, 'open', $3)", invoice_id, uid, time.Now())
	} else {
		res.Scan(&invoice_id)
		res.Close()
	}
	count--
	app.DB.Exec("UPDATE book SET quantity_sale=$1 WHERE book_id=$2", count, bid)
	app.DB.Exec("INSERT INTO invoice_book(invoice_id, book_id) VALUES($1, $2)", invoice_id, bid)
	c.JSON(http.StatusOK, gin.H{"message": "book added to cart"})
}

func (app *App) DeleteFromCart(c *gin.Context) {
	bid, _ := strconv.Atoi(c.Param("bookid"))
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT invoice_id FROM invoice WHERE user_id=$1 AND status='open'", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !res.Next() {
		c.JSON(http.StatusNotFound, gin.H{"message": "no active invoices"})
		return
	}
	var invoice_id int
	res.Scan(&invoice_id)
	res.Close()
	res, err = app.DB.Query("SELECT quantity_sale FROM book WHERE book_id = $1", bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	res.Next()
	var count int
	res.Scan(&count)
	count++
	app.DB.Exec("UPDATE book SET quantity_sale=$1 WHERE book_id=$2", count, bid)
	app.DB.Exec("DELETE FROM invoice_book WHERE invoice_id=$1 AND book_id=$2", invoice_id, bid)
	c.JSON(http.StatusOK, gin.H{"message": "book removed from cart"})
}

func (app *App) IsInCart(c *gin.Context) {
	bid, _ := strconv.Atoi(c.Param("bookid"))
	uid := functions.GetUserId(c.GetHeader("Authorization"))
	res, err := app.DB.Query("SELECT * FROM invoice INNER JOIN invoice_book ON invoice.invoice_id=invoice_book.invoice_id WHERE invoice.user_id=$1 AND invoice_book.book_id=$2 AND invoice.status='open'", uid, bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	if !res.Next() {
		c.JSON(http.StatusNotFound, gin.H{"message": "no such book on active invocie"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "book exists in active invoice"})
}

func (app *App) GetActiveInvoice(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("authorization"))
	res, err := app.DB.Query("SELECT book.book_id, book.price, book.title, book.image_url FROM invoice INNER JOIN invoice_book ON invoice.invoice_id = invoice_book.invoice_id INNER JOIN book ON book.book_id = invoice_book.book_id WHERE invoice.status = 'open' AND invoice.user_id = $1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	books := []models.LowBook{}
	for res.Next() {
		var book models.LowBook
		res.Scan(&book.Id, &book.Price, &book.Title, &book.ImageUrl)
		books = append(books, book)
	}
	if len(books) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "no active invoices"})
		return
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) FinalizeInvoice(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("authorization"))
	res, err := app.DB.Query("SELECT invoice_id FROM invoice WHERE user_id=$1 AND status='open'", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !res.Next() {
		c.JSON(http.StatusNotFound, gin.H{"message": "no active invoices found for user"})
		return
	}
	var iid int
	res.Scan(&iid)
	app.DB.Exec("UPDATE invoice SET status='close', purchase_date=$2 WHERE invoice_id=$1", iid, time.Now())
	res.Close()
	res, _ = app.DB.Query("SELECT book_id FROM invoice_book WHERE invoice_id=$1", iid)
	defer res.Close()
	for res.Next() {
		var bid int
		res.Scan(&bid)
		app.DB.Exec("INSERT INTO user_read(userid, book_id) VALUES($1, $2)", uid, bid)
	}
	link := fmt.Sprintf("https://bikaransystem.work.gd/invoice/%d", iid)
	subject := "سفارش شما تکمیل شد"
	body := fmt.Sprintf(`<p>سفارش شما به شماره %d تکمیل شد</p>
	<p>برای دریافت سفارش کافی است در روز های آتی به کتابخانه مراجعه نمایید.</p>
	<a href="%s">نمایش سفارش</a>`, iid, link)
	res, _ = app.DB.Query("SELECT email FROM users WHERE user_id=$1", uid)
	res.Next()
	var email string
	res.Scan(&email)
	functions.SendEmail(email, subject, body, app.Email)
	c.JSON(http.StatusOK, gin.H{"message": "invoice closed"})
}

func (app *App) ShowInvoice(c *gin.Context) {
	iid, _ := strconv.Atoi(c.Param("invoice"))
	res, err := app.DB.Query("SELECT book.book_id, book.price, book.title, book.image_url FROM invoice INNER JOIN invoice_book ON invoice.invoice_id=invoice_book.invoice_id INNER JOIN book ON book.book_id=invoice_book.book_id WHERE invoice.invoice_id=$1", iid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	books := []models.LowBook{}
	for res.Next() {
		var book models.LowBook
		res.Scan(&book.Id, &book.Price, &book.Title, &book.ImageUrl)
		books = append(books, book)
	}
	c.JSON(http.StatusOK, books)
}

func (app *App) InvoiceHistory(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("authorization"))
	res, err := app.DB.Query("SELECT invoice_id, purchase_date FROM invoice WHERE user_id=$1 AND status='close'", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	var invoices []struct {
		InvoiceID    int       `json:"invoice_id"`
		PurchaseDate time.Time `json:"purchase_date"`
	}
	for res.Next() {
		var invoice struct {
			InvoiceID    int       `json:"invoice_id"`
			PurchaseDate time.Time `json:"purchase_date"`
		}
		res.Scan(&invoice.InvoiceID, &invoice.PurchaseDate)
		invoices = append(invoices, invoice)
	}
	if len(invoices) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "no invoices found for user"})
		return
	}
	c.JSON(http.StatusOK, invoices)
}

func (app *App) CustomerInvoiceHistory(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("authorization"))
	res, err := app.DB.Query("SELECT role from users WHERE user_id=$1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.AbortWithStatus(http.StatusNotAcceptable)
		return
	}
	res.Close()
	res, err = app.DB.Query("SELECT invoice_id, purchase_date, (firstname || ' ' || lastname) as name FROM invoice INNER JOIN users on users.user_id=invoice.user_id WHERE status='close'", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer res.Close()
	var invoices []struct {
		InvoiceID    int       `json:"invoice_id"`
		PurchaseDate time.Time `json:"purchase_date"`
		Name         string    `json:"customer_name"`
	}
	for res.Next() {
		var invoice struct {
			InvoiceID    int       `json:"invoice_id"`
			PurchaseDate time.Time `json:"purchase_date"`
			Name         string    `json:"customer_name"`
		}
		res.Scan(&invoice.InvoiceID, &invoice.PurchaseDate, &invoice.Name)
		invoices = append(invoices, invoice)
	}
	if len(invoices) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "no invoices found"})
		return
	}
	c.JSON(http.StatusOK, invoices)
}

func (app *App) IsAdmin(c *gin.Context) {
	uid := functions.GetUserId(c.GetHeader("authorization"))
	res, err := app.DB.Query("SELECT role from users WHERE user_id=$1", uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": err.Error()})
		return
	}
	res.Next()
	var b bool
	res.Scan(&b)
	if !b {
		c.JSON(http.StatusNotAcceptable, gin.H{"message": "not admin"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "is admin"})
}
