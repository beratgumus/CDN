package main

import (
	"github.com/buaazp/fasthttprouter"
	"github.com/valyala/fasthttp"
	"log"
	"bytes"
	"strconv"
	"image"
	"os"
	"image/png"
	"image/jpeg"
	"github.com/disintegration/imaging"
	"image/color"
	"time"
	"math/rand"
	"net/http"
	"strings"
)

//---- sunucu ayarları -----
var cacheImages = true // orijinal resim önbelleğini açar/kapatır
var cacheResponses = false  // değişiklik yapılmış resim önbelleğini açar/kapatır
var remoteUrl = "http://bihap.com/img/"
var readLocal = false
var serverAddr = ":8080"

var resp = []byte("<h1>Hello World</h1>")
var buffer = new(bytes.Buffer)
var imageCache = make(map[string]image.Image)
var respCache = make(map[string][]byte)
var lockRespCache = false
var lockImgCache = false
var quality = 75 // resim kalitesi, 1 - 100

func main() {

	r := fasthttprouter.New()

	//fs := &fasthttp.FS{
	//	Root:               "./public",
	//	IndexNames:         []string{"index.html"},
	//	GenerateIndexPages: true,
	//	Compress:           false,
	//	AcceptByteRange:    false,
	//}

	//fsHandler := fs.NewRequestHandler()

	//r.GET("/", fileServing)

	r.GET("/", func(ctx *fasthttp.RequestCtx){
		ctx.Redirect("/home", 301)
	})
	//r.GET("/home/*filepath", fsHandler)
	r.GET("/path", fhttpHandler)
	r.GET("/img/:fileName", imagingHandler)
	r.GET("/img", imagingHandler)

	r.NotFound = errorHandler

	rand.Seed(time.Now().UTC().UnixNano())
	fasthttp.ListenAndServe(serverAddr, r.Handler)
}

func errorHandler( ctx *fasthttp.RequestCtx )  {
	log.Print(ctx.Request.URI())
}


func fileServing(ctx *fasthttp.RequestCtx) {
	fasthttp.ServeFile(ctx, "./public")
}

func fhttpHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(200)
	ctx.Write(resp)
}


// --- geçerli istek listesi ---
// http://localhost/img/hello.png?x=200
// http://localhost/img/hello.png?y=300
// http://localhost/img/hello.png?x=100&y=300
// http://localhost/img/hello.png?x=100&y=300&crop
//
// parametreler: x, y, color, crop
//
// resmi, verilen parametrelere göre şekillendiren fonksiyon
func imagingHandler(ctx *fasthttp.RequestCtx){
	ctx.SetStatusCode(200) // her zaman 200 kodu

	//log.Print(ctx.Request.URI())

	uri := ctx.URI().String()

	var exists bool // önbellek kontrolü için (map içinde var mı yok mu) gerekli değişken

	if cacheResponses {
		// TODO: use mutex
		// NOT: bu yöntem mantıklı değil. bir çok farklı istekde RAM kullanımı çok artar

		var respImg []byte
		respImg, exists = respCache[uri]

		if exists {
			// bu istek önbellekte var. daha önceden üretilmiş sonucu hemen döndürelim
			ctx.SetContentType("image/jpeg")
			ctx.Write(respImg)
			return
		}
	}

	//  /img/:fileName  ->  /img/hello.png  -> fileName = hello.png

	var fileName string
	if ctx.UserValue("fileName") != nil {
		// bilgisayarımızdaki dosyayı açacağız
		fileName = ctx.UserValue("fileName").(string)
	} else {
		ctx.SetContentType("text/html; charset=utf-8")
		ctx.Write([]byte("<b>Hata:</b> Resim dosyası seçilmedi/girilmedi."))
		return
	}
	//log.Printf(fileName)

	var img image.Image
	var err error

	if cacheImages {
		// işleyeceğimiz dosya önbellekte var mı bakalım
		img, exists = imageCache[fileName]

		if !exists {
			// yokmuş. hemen yükleyelim
			img, err = loadImage(fileName)

			if err != nil {
				ctx.SetContentType("text/html; charset=utf-8")
				ctx.Write([]byte("<b>Hata:</b> Dosya bulunamadı."))
				return
			}

			if !lockImgCache {
				// TODO: use mutex
				lockImgCache = true
				imageCache[fileName] = img
				//log.Printf("writed %s to image cache", fileName)
				lockImgCache = false
			}

		}
	} else {
		img, err = loadImage(fileName)
		if err != nil {
			ctx.SetContentType("text/html; charset=utf-8")
			ctx.Write([]byte("<b>Hata:</b> Dosya bulunamadı."))
			return
		}
	}

	colorStr := "" // gray, red, blue, random

	x := 0  // genişlik, 1 - 2000
	y := 0  // yükseklik, 1 - 2000

	isXExists := false
	isYExists := false


	if ctx.QueryArgs().Has("width") {
		isXExists = true

		// ?x=54 vey ?x=asd gibi bir şey girilmiş, değeri okuyup integer'a çevirelim
		x, err = strconv.Atoi(string(ctx.QueryArgs().Peek("width")))

		// x bir sayı değilse veya 1 den küçük 2000den büyükse
		if err != nil || x < 1 || x > 20000 {
			ctx.SetContentType("text/html; charset=utf-8")
			ctx.Write([]byte("<b>Hata:</b> Genişlik (x) 1 ile 2000 arasında bir tam sayı olmalıdır"))
			return
		}
	}

	if ctx.QueryArgs().Has("height") {
		isYExists = true

		// ?y=154 vey ?x=afdv gibi bir şey girilmiş, değeri okuyup integer'a çevirelim
		y, err = strconv.Atoi(string(ctx.QueryArgs().Peek("height")))

		if err != nil || y < 1 || y > 20000 {
			ctx.SetContentType("text/html; charset=utf-8")
			ctx.Write([]byte("<b>Hata:</b> Yükseklik (y) 1 ile 2000 arasında bir tam sayı olmalıdır."))
			return
		}
	}


	if ctx.QueryArgs().Has("color") {
		colorStr = strings.ToLower(string(ctx.QueryArgs().Peek("color")))
	}

	//log.Printf("x: %s - y: %s", img.Bounds().Dx(), img.Bounds().Dy())

	if isXExists || isYExists {

		xResizeRatio := img.Bounds().Dx() / x
		yResizeRatio := img.Bounds().Dy() / y

		//x'deki değişim miktarı y'deki değişim miktarından büyükse x'e göre yeniden boyutlandırılır
		// orijinal : 	600x400  	600x400
		// istenilen : 	300x100 	100x100
		// çıktı :		300x200		150x100
		if xResizeRatio > yResizeRatio {
			// imaging.NearestNeighbor en hızlı yeniden boyutlandırma yöntemidir.
			img = imaging.Resize(img, 0, y, imaging.NearestNeighbor)
		} else {
			img = imaging.Resize(img, x, 0, imaging.NearestNeighbor)
		}
	}

	if colorStr != "" {

		if colorStr == "gray" {
			img = imaging.Grayscale(img)
		} else if colorStr == "red" {

			//her bir pixel'e bu fonksiyon uygulanır.
			img = imaging.AdjustFunc( img, func(c color.NRGBA) color.NRGBA {

				//c değişkeni orijinal pixelin rengini tutar.
				return color.NRGBA{c.R, 0, 0, c.A}
			})

		} else if colorStr == "green" {
			img = imaging.AdjustFunc( img, func(c color.NRGBA) color.NRGBA {
				return color.NRGBA{0, c.G, 0, c.A}
			})

		} else if colorStr == "blue" {
			img = imaging.AdjustFunc( img, func(c color.NRGBA) color.NRGBA {
				return color.NRGBA{0, 0, c.B, c.A}
			})

		}


		//colorFilter := imaging.New(1000, 1000, color.Gray{} )
		//img = imaging.OverlayCenter(img, colorFilter, 0.5)

	}

	//err = imaging.Save(img, "public/images/out/flowers.jpg")
	//if err != nil {
	//	log.Printf("failed to save image: %v", err)
	//}

	// resmi encode edelim. encode edilen resim bufferın içinde tutulur.
	// daha sonra bufferdaki veriyi okuyup yanıtımızın içine koyacağız.
	err = jpeg.Encode(buffer, img, &jpeg.Options{ Quality: quality})
	if  err != nil {
		log.Println("unable to encode image.")
	}


	//ctx.SetContentType("text/html; charset=utf-8")
	//ctx.Write([]byte("Resim oluşturuldu!"))

	resp = buffer.Bytes()

	// yanıtlar önbelleğe alınacaksa ve önbelleğe başka bir istek içinden
	// erişilmiyorsa yazma işlemi yapılır. rastgele oluşturulan resimler asla önbelleğe alınmaz
	if cacheResponses && !lockRespCache{
		// önbelleğe yazacağız. farklı threadlerden aynı anda yazma
		// işlemi yapmamak için bu kiliti aktifleştirelim
		lockRespCache = true
		respCache[uri] = resp
		//log.Printf("writed %s to response cache", uri)
		lockRespCache = false
		//go writeToCache(uri, resp)
	}


	ctx.SetContentType("image/jpeg")
	ctx.Write(resp)
	buffer.Reset()
}


func loadImage(filename string) (image.Image, error){
	var img image.Image
	var err error

	if strings.Contains(filename, "http") {
		// başka bir sunucudan resim çekeceğiz
		response, _ := http.Get(filename)
		defer response.Body.Close()
		img, _, err = image.Decode(response.Body)
	} else if !readLocal {
		response, _ := http.Get(remoteUrl + filename)
		defer response.Body.Close()
		img, _, err = image.Decode(response.Body)
		if err != nil {
			log.Printf("Remote image fetch failed: %v", err)
			return nil, err
		}
		//log.Printf("opened image from url: %s%s", remoteUrl, filename)
	} else {

		f, err := os.Open("public/images/" + filename)
		if err != nil {
			log.Printf("os.Open failed: %v", err)
			return nil, err
		}

		img, _, err = image.Decode(f)
		f.Close()
	}

	if err != nil {
		log.Printf("image.Decode failed: %v", err)
		return nil, err
	}

	return img, nil
}

func saveImage(filename string, img image.Image) {
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("os.Create failed: %v", err)
	}
	err = png.Encode(f, img)
	if err != nil {
		log.Fatalf("png.Encode failed: %v", err)
	}
}





// GIN

//func testHandler(ctx *gin.Context) {
//	fileName := ctx.Param("fileName")
//
//	content := make(map[string]string)
//
//	content["host"] = ctx.Request.Host
//	for i, param := range ctx.Params {
//		content["param" + string(i)] = param.Value
//	}
//
//	content["ip"] = ctx.ClientIP()
//	content["x"] = ctx.Query("x")
//	content["y"] = ctx.Query("y")
//
//
//	ctx.HTML(200, "no-sidebar.tmpl", gin.H{
//		"title": "Website Title",
//		"number": fileName,
//		"content": content,
//	})
//}
