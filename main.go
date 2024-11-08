package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/meynay/BookStore/handlers"
)

func getDB() *sql.DB {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}
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
	app := handlers.App{
		DB: getDB(),
	}
	engine := gin.Default()
	engine.GET("/getbooks", app.GetBooks)
	engine.GET("/getbook/:id", app.GetBook)
	engine.GET("/newbooks", app.GetNewBooks)
	engine.GET("/filterbooks", app.FilterBooks)
	engine.POST("/login", app.Login)
	engine.POST("/signup", app.Signup)
	engine.Use(app.AuthMiddleware())
	{
		engine.GET("/userprofile", app.GetUserProfile)
		engine.GET("/recommendbooksbyrecord", app.RecommendByRecord)
		engine.GET("/recommendbooksbyrate", app.RecommendByRates)
		engine.POST("/addbook", app.AddBook)
		engine.PUT("/editbook", app.EditBook)
		engine.GET("/favecheck", app.CheckIfFaved)
		engine.POST("/fave", app.FaveOrUnfave)
		engine.POST("/ratebook", app.RateBook)
	}
	port := os.Getenv("PORT")
	host := os.Getenv("HOST")
	engine.Run(host + port)
}
