package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/image-duplication-service/docs"

	"github.com/cloudinary/cloudinary-go"
	"github.com/cloudinary/cloudinary-go/api/uploader"
	"github.com/corona10/goimagehash"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nfnt/resize"
	swaggerFiles "github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"
)

type ImageMeta struct {
	ID       int
	Hash     int64
	HashType string
	Url      string
	Metadata Metadata `json:"metadata"`
}

type Metadata struct {
	NamaPetani string
	Alamat     string
	Kota       string
}

var db *sql.DB

func main() {
	// load .env file
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable", os.Getenv("HOST"), os.Getenv("PORT"), os.Getenv("POSTGRESQL_USER"), os.Getenv("POSTGRESQL_PASSWORD"), os.Getenv("DBNAME"))

	fmt.Println(psqlInfo)
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	defer db.Close()

	docs.SwaggerInfo.Title = "Swagger Image Duplication API"
	docs.SwaggerInfo.Description = "This is a sample server image dupication server."
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.Host = "localhost:8081"
	docs.SwaggerInfo.Schemes = []string{"http", "https"}

	router := gin.Default()
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.POST("/check", checkDuplicate)
	router.Run(":8081")
}

// @Summary Check for duplicate images
// @Description Check if an image is a duplicate by comparing its Perceptual hash with existing hashes in the database
// @Accept  multipart/form-data
// @Produce json
// @Param   image     formData   file    true   "Image file to upload"
// @Param   nama_petani     formData   string    true   "Petani's name"
// @Param   alamat     formData   string    true   "Address"
// @Param   kota     formData   string    true   "City"
// @Success 200 {object} string "Returns the public URL of the uploaded image"
// @Success 202 {object} string "Returns the error and URL of the image if already exists"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /check [post]
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

	img1 := resize.Resize(256, 0, content, resize.Bicubic)
	grayImg := image.NewGray(img1.Bounds())
	for x := grayImg.Bounds().Min.X; x < grayImg.Bounds().Max.X; x++ {
		for y := grayImg.Bounds().Min.Y; y < grayImg.Bounds().Max.Y; y++ {
			grayImg.Set(x, y, img1.At(x, y))
		}
	}

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

	hash := imageHash.GetHash()

	datas, err := getImageByHash(hash)

	// image already found, return error, return url
	if err == nil {
		c.JSON(http.StatusAccepted, gin.H{
			"error":    "image already exist",
			"metadata": datas.Metadata,
			"url":      datas.Url,
		})
		return
	}

	if err != sql.ErrNoRows {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// upload image if hash is not exist
	url, err := uploadImage(c)

	// create metadata
	metadata := Metadata{
		NamaPetani: c.Request.FormValue("nama_petani"),
		Alamat:     c.Request.FormValue("alamat"),
		Kota:       c.Request.FormValue("kota"),
	}
	// save to db
	imgMeta := ImageMeta{
		Hash:     int64(hash),
		HashType: "PerceptualHash",
		Url:      url,
		Metadata: metadata,
	}

	res, err := saveData(imgMeta)

	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// Return the public URL of the uploaded image
	c.JSON(http.StatusOK, res)
}

func getImageByHash(hash uint64) (*ImageMeta, error) {
	var img ImageMeta
	var metadata string
	// var metadataJSON []byte
	err := db.QueryRow("SELECT id, hash, hash_type, url, metadata FROM image_meta WHERE hash = $1", int64(hash)).Scan(&img.ID, &img.Hash, &img.HashType, &img.Url, &metadata)

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(metadata), &img.Metadata)
	if err != nil {
		return nil, err
	}

	return &img, nil
}

func uploadImage(c *gin.Context) (string, error) {
	file, err := c.FormFile("image")
	if err != nil {
		return "", err
	}

	src, err := file.Open()
	if err != nil {
		return "", err
	}

	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//create cloudinary instance
	cld, err := cloudinary.NewFromParams(os.Getenv("CLOUDINARY_CLOUD_NAME"), os.Getenv("CLOUDINARY_API_KEY"), os.Getenv("CLOUDINARY_API_SECRET"))
	if err != nil {
		return "", err
	}

	//upload file
	uploadParam, err := cld.Upload.Upload(ctx, src, uploader.UploadParams{Folder: os.Getenv("CLOUDINARY_UPLOAD_FOLDER")})
	if err != nil {
		c.String(http.StatusInternalServerError, "error di sini kah?")
		return "", err
	}

	return uploadParam.SecureURL, nil
}

func saveData(image ImageMeta) (*ImageMeta, error) {
	// Query database for similar images
	var metadata string
	sql := "INSERT INTO image_meta (hash, hash_type, url, metadata) VALUES($1, $2, $3, $4) RETURNING id, hash, hash_type, url, metadata"

	metadataJSON, err := json.Marshal(image.Metadata)
	if err != nil {
		return nil, err
	}

	err = db.QueryRow(sql, image.Hash, image.HashType, image.Url, metadataJSON).Scan(&image.ID, &image.Hash, &image.HashType, &image.Url, &metadata)
	err = json.Unmarshal([]byte(metadata), &image.Metadata)

	result := &image

	if err != nil {
		return nil, err
	}

	return result, nil
}
