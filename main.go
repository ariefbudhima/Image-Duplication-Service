package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cloudinary/cloudinary-go"
	"github.com/cloudinary/cloudinary-go/api/uploader"
	"github.com/corona10/goimagehash"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nfnt/resize"
)

type ImageMeta struct {
	ID       int
	Hash     string
	HashType string
	Url      string
}

var db *sql.DB

func main() {
	// load .env file
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// connect to db
	// db, err := sql.Open("postgres://myuser:mypassword@localhost/mydb?sslmode=disable")
	// if err != nil {
	// 	// Handle error
	// }

	db, err = sql.Open("postgres", "host=localhost port=5432 user=myuser password=mypassword dbname=mydb sslmode=disable")
	if err != nil {
		panic(err)
	}

	defer db.Close()

	router := gin.Default()
	router.POST("/check", uploadImage)
	// router.GET("/search", searchHandler)
	router.Run(":8081")
}

func checkDuplicate(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		c.String(http.StatusBadRequest, "Bad Request")
		return
	}

	src, err := file.Open()
	if err != nil {
		c.String(http.StatusBadRequest, "Cannot open File")
		return
	}

	defer src.Close()

	// Check image type
	buff := make([]byte, 512)
	_, err = src.Read(buff)
	if err != nil {
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	contentType := http.DetectContentType(buff)
	if contentType != "image/png" && contentType != "image/jpeg" {
		c.String(http.StatusBadRequest, "Unsupported image type")
		return
	}

	// Decode image
	src.Seek(0, 0) // rewind to the beginning of the file
	var content image.Image
	var er error
	if contentType == "image/png" {
		content, er = png.Decode(src)
	} else if contentType == "image/jpeg" {
		content, er = jpeg.Decode(src)
	}

	if er != nil {
		c.String(http.StatusBadRequest, "Cannot decode image")
		return
	}

	// Resize gambar agar ukurannya seragam
	img1 := resize.Resize(256, 0, content, resize.Lanczos3)

	// Hitung Perceptual hash dari gambar
	imageHash, err := goimagehash.PerceptionHash(img1)
	if err != nil {
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	// Compute SHA-256 hash of image
	h := sha256.New()
	_, err = src.Seek(0, 0)
	if err != nil {
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}
	if _, err := io.Copy(h, src); err != nil {
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}
	sha256hash := hex.EncodeToString(h.Sum(nil))

	hash := imageHash.GetHash()
	var data2 uint64
	data2 = 13893096327330782963
	hash2 := goimagehash.NewImageHash(data2, goimagehash.PHash)
	// chech hash of the image to database, if any, return error

	fmt.Println("===============================================================================")
	fmt.Println("sha256hash : ", sha256hash)
	fmt.Println("hash : ", imageHash.ToString())
	fmt.Println("perceptualHash : ", hash)

	imgs, _ := imageHash.Distance(hash2)

	c.JSON(http.StatusOK, gin.H{"result": imgs})
}

func uploadImage(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		c.String(http.StatusBadRequest, "Bad Request")
		return
	}

	src, err := file.Open()
	if err != nil {
		c.String(http.StatusBadRequest, "Cannot open file")
		return
	}

	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//create cloudinary instance
	cld, err := cloudinary.NewFromParams(os.Getenv("CLOUDINARY_CLOUD_NAME"), os.Getenv("CLOUDINARY_API_KEY"), os.Getenv("CLOUDINARY_API_SECRET"))
	if err != nil {
		c.String(http.StatusInternalServerError, "cannot create cloudinary new form param")
		return
	}

	//upload file
	uploadParam, err := cld.Upload.Upload(ctx, src, uploader.UploadParams{Folder: os.Getenv("CLOUDINARY_UPLOAD_FOLDER")})
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// save to db
	imgMeta := ImageMeta{
		Hash:     "121212",
		HashType: "blabla",
		Url:      uploadParam.SecureURL,
	}

	res, err := saveData(imgMeta)

	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// Return the public URL of the uploaded image
	c.JSON(http.StatusOK, res)
}

func saveData(image ImageMeta) (*ImageMeta, error) {
	// Query database for similar images
	sql := "INSERT INTO image_meta (hash, hash_type, url) VALUES($1, $2, $3) RETURNING id, hash, hash_type, url"

	err := db.QueryRow(sql, image.Hash, image.HashType, image.Url).Scan(&image.ID, &image.Hash, &image.HashType, &image.Url)
	result := &image

	if err != nil {
		return nil, err
	}

	fmt.Println("=======================================================================")
	fmt.Println(result)
	return result, nil
}
