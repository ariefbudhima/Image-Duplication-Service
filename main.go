package main

import (
	"context"
	"crypto/sha256"
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
	"github.com/nfnt/resize"
)

var (
	hashList = make(map[string]string)
)

func main() {
	// load .env file
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	//create cloudinary instance
	cld, err := cloudinary.NewFromParams(os.Getenv("CLOUDINARY_CLOUD_NAME"), os.Getenv("CLOUDINARY_API_KEY"), os.Getenv("CLOUDINARY_API_SECRET"))
	if err != nil {
		c.String(http.StatusInternalServerError, "cannot create cloudinary new form param")
		return
	}

	fmt.Println("=============================================================")
	fmt.Println(os.Getenv("CLOUDINARY_CLOUD_NAME"))
	fmt.Println(os.Getenv("CLOUDINARY_API_KEY"))
	fmt.Println(os.Getenv("CLOUDINARY_API_SECRET"))

	//upload file
	uploadParam, err := cld.Upload.Upload(ctx, src, uploader.UploadParams{Folder: os.Getenv("CLOUDINARY_UPLOAD_FOLDER")})
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	// return uploadParam.SecureURL, nil

	// Return the public URL of the uploaded image
	c.JSON(http.StatusOK, gin.H{
		"url": uploadParam.SecureURL,
	})
}

// how to connect
