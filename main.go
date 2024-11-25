package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/meynay/BookStore/handlers"
	"github.com/meynay/BookStore/models"
)

func getDB() *sql.DB {
	port := os.Getenv("DB_PORT")
	database := os.Getenv("DB_DB")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	host := os.Getenv("DB_HOST")
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable", host, user, pass, database, port)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Error connecting to database")
	}
	return db
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}
	app := handlers.App{
		DB: getDB(),
		Email: models.EmailConfig{
			SMTPHost:    "smtp.gmail.com",
			SMTPPort:    587,
			Username:    os.Getenv("SMTP_USERNAME"),
			Password:    os.Getenv("SMTP_PASSWORD"),
			SenderEmail: os.Getenv("SMTP_USERNAME"),
		},
		ResetToken: make(map[string]string),
	}
	engine := gin.Default()
	engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // Change to your domain
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "x-api-key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	engine.Use(app.ApiKeyCheck())
	{
		//user sign in/up apis
		engine.POST("/login", app.Login)
		engine.POST("/signup", app.Signup)

		//reset password apis
		engine.POST("/tryresetpassword", app.ResetPasswordMail)
		engine.POST("/resetpassword/:token", app.ResetPassword)

		//get books apis
		engine.GET("/getbooks", app.GetBooks)
		engine.GET("/newbooks", app.GetNewBooks)
		engine.GET("/filterbooks", app.FilterBooks)

		//single book apis
		engine.GET("/getbook/:id", app.GetBook)
		engine.GET("/rates/:book_id", app.GetRates)
		engine.GET("/comments/:book_id", app.GetComments)

		engine.Use(app.AuthMiddleware())
		{
			//user profile apis
			engine.GET("/userinfo", app.GetUserInfo)
			engine.GET("/userprofile", app.GetUserProfile)
			engine.GET("/image/:image", app.GetProfPic)
			engine.POST("/userimageupload", app.UploadImage)

			//recommenders apis
			engine.GET("/recommendbooksbyrecord", app.RecommendByRecord)
			engine.GET("/recommendbooksbyrate", app.RecommendByRates)

			//user actions on books apis
			engine.GET("/favecheck/:book_id", app.CheckIfFaved)
			engine.POST("/fave", app.FaveOrUnfave)
			engine.POST("/ratebook", app.RateBook)
			engine.POST("/commentbook", app.CommentOnBook)
			engine.GET("/getfavebooks", app.GetFavedBooks)

			//borrow book apis
			engine.GET("/libstatus/:bookid", app.GetLibStatus)
			engine.POST("/borrowbook/:bookid", app.BorrowBook)
			engine.GET("/borrowhistory", app.BorrowHistory)

			//buy books
			engine.POST("/addtocart/:bookid", app.AddToCart)
			engine.DELETE("/deletefromcart/:bookid", app.DeleteFromCart)
			engine.GET("/incart/:bookid", app.IsInCart)
			engine.GET("/activeinvoice", app.GetActiveInvoice)
			engine.POST("/finalizeinvoice", app.FinalizeInvoice)
			engine.GET("/showinvoice/:invoice", app.ShowInvoice)
			engine.GET("/invoicehistory", app.InvoiceHistory)

			//logout api
			engine.POST("/logout", app.Logout)

			//administrative apis
			engine.GET("/borrowedbooks", app.ShowActiveBorrows)
			engine.POST("/returnbook/:bookid", app.ReturnBook)
			engine.GET("/customerinvoices", app.CustomerInvoiceHistory)
			//book changes apis
			engine.POST("/addbook", app.AddBook)
			engine.PUT("/editbook", app.EditBook)
		}
	}
	port := os.Getenv("PORT")
	host := os.Getenv("HOST")
	engine.Run(host + port)
}
