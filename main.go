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



var resp = []byte("<h1>This is response</h1>")
var buffer = new(bytes.Buffer)
var imageCache = make(map[string]image.Image)
var respCache = make(map[string][]byte)
var lockRespCache = false
var lockImgCache = false

func main() {

	r := fasthttprouter.New()

	fs := &fasthttp.FS{
		Root:               "./public",
		IndexNames:         []string{"index.html"},
		GenerateIndexPages: true,
		Compress:           false,
		AcceptByteRange:    false,
	}

	fsHandler := fs.NewRequestHandler()

	//r.GET("/", fileServing)

	r.GET("/", func(ctx *fasthttp.RequestCtx){
		ctx.Redirect("/home", 301)
	})
	r.GET("/home/*filepath", fsHandler)
	r.GET("/path", fhttpHandler)
	r.GET("/img/:fileName", imagingHandler)
	r.GET("/img", imagingHandler)

	//imageCache["flowers.png"] = loadImage("public/images/flowers.png")
	//imageCache["pic04.jpg"] = loadImage("public/images/pic04.jpg")

	rand.Seed(time.Now().UTC().UnixNano())

	fasthttp.ListenAndServe(":80", r.Handler)

}

func randInt() uint8 {
	return uint8(rand.Intn(255))
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
// http://localhost/img/hello.png?x=200&y=300&crop&color=gray&quality=50&blur=3.5
//
// http://localhost/img?url=https://bekiruzun.com/images/post/localhost.jpg&x=300&y=200&quality=70
//
// parametreler: x, y, color, crop, quality, blur, url
//
// resmi, verilen parametrelere göre şekillendiren fonksiyon
func imagingHandler(ctx *fasthttp.RequestCtx){
	ctx.SetStatusCode(200) // her zaman 200 kodu

	uri := ctx.URI().String()

	var exists bool // önbellek kontrolü için (map içinde var mı yok mu) gerekli değişken

	if cacheResponses {
		// NOT: bu yöntem mantıklı değil. bir çok farklı istekde RAM kullanımı çok artar
		// Fakat benchmarklarda harika performans artışı oluyor :)

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
	} else if ctx.QueryArgs().Has("url"){
		// uzak bir sunucudaki resmi açacağız
		fileName = string(ctx.QueryArgs().Peek("url"))
		//log.Printf("url=%s", fileName)
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
				lockImgCache = true
				imageCache[fileName] = img
				log.Printf("writed %s to image cache", fileName)
				lockImgCache = false
			}

		}
	} else {
		img, _ = loadImage(fileName)
	}

	colorStr := "" // gray, red, blue, random
	crop := false
	x := 0  // genişlik, 1 - 2000
	y := 0  // yükseklik, 1 - 2000
	quality := 75 // resim kalitesi, 1 - 100
	blur := 0.0  // bulanıklık, 0 - 25
	isRand := false  //rastgele renk mi?
	isXExists := false
	isYExists := false


	if ctx.QueryArgs().Has("x") {
		isXExists = true

		// ?x=54 vey ?x=asd gibi bir şey girilmiş, değeri okuyup integer'a çevirelim
		x, err = strconv.Atoi(string(ctx.QueryArgs().Peek("x")))

		// x bir sayı değilse veya 1 den küçük 2000den büyükse
		if err != nil || x < 1 || x > 2000 {
			ctx.SetContentType("text/html; charset=utf-8")
			ctx.Write([]byte("<b>Hata:</b> Genişlik (x) 1 ile 2000 arasında bir tam sayı olmalıdır"))
			return
		}
	}

	if ctx.QueryArgs().Has("y") {
		isYExists = true

		// ?y=154 vey ?x=afdv gibi bir şey girilmiş, değeri okuyup integer'a çevirelim
		y, err = strconv.Atoi(string(ctx.QueryArgs().Peek("y")))

		if err != nil || y < 1 || y > 2000 {
			ctx.SetContentType("text/html; charset=utf-8")
			ctx.Write([]byte("<b>Hata:</b> Yükseklik (y) 1 ile 2000 arasında bir tam sayı olmalıdır."))
			return
		}
	}

	// tam olarak verilen boyuta mı çevrilecek?
	// orijinal resim 1000x250 ise x=100 y=100 ise, crop url'ye girilmemişse
	// çıktı 100x50 olur (oran bozulmaz). Crop url'de varsa çıktı 100x100 olur
	if ctx.QueryArgs().Has("crop") {
		crop = true
	}

	if ctx.QueryArgs().Has("quality") {
		quality, err = strconv.Atoi(string(ctx.QueryArgs().Peek("quality")))

		if err != nil || quality < 1 || quality > 100 {
			ctx.SetContentType("text/html; charset=utf-8")
			ctx.Write([]byte("<b>Hata:</b> Çözünürlük 1 ile 100 arasında bir tam sayı olmalıdır."))
			return
		}
	}

	if ctx.QueryArgs().Has("color") {
		colorStr = string(ctx.QueryArgs().Peek("color"))
	}

	if ctx.QueryArgs().Has("blur") {
		blur, err = strconv.ParseFloat(string(ctx.QueryArgs().Peek("blur")), 64)

		if err != nil || blur <= 0 || blur > 50 {
			ctx.SetContentType("text/html; charset=utf-8")
			ctx.Write([]byte("<b>Hata:</b> Bulanıklık 0 ile 50 arasında bir sayı olmalıdır."))
			return
		}
	}

	//log.Printf("x=%d y=%d", x, y)


	// resim kırpılacaksa hem x hem de y değeri girilmiş olmalıdır
	if (isXExists != isYExists) && crop {
		ctx.SetContentType("text/html; charset=utf-8")
		ctx.Write([]byte("<b>Hata:</b> Kırpma işlemi için x ile y değeri birlikte verilmelidir."))
		return
	}

	if isXExists || isYExists {

		// x büyükse y sabit tutulur, y büyükse x sabit tutulur.
		// bu sayede orijinal resmin en/boy oranı değişmez.
		if x > y {
			// imaging.NearestNeighbor en hızlı yeniden boyutlandırma yöntemidir.
			img = imaging.Resize(img, x, 0, imaging.NearestNeighbor)
		} else {
			img = imaging.Resize(img, 0, y, imaging.NearestNeighbor)
		}
	}

	if crop {
		//resmi ortalayarak kırpalım
		img = imaging.CropAnchor(img, x, y, imaging.Center)
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

		} else {
			r1 := randInt()
			r2 := randInt()

			// NOT: önemsiz bir fantezi
			img = imaging.AdjustFunc( img, func(c color.NRGBA) color.NRGBA {
				// c.R, c.G, C.B değişkenleri pixelin orijinal renkleridir
				// her bir pixelin son renk durumu r, g, b değişkenindedir
				var r, g, b uint8

				if r1 < 85 {
					if r1 < 42 {
						r = c.R - r2
					} else {
						r = c.R + r2
					}
					g = c.G
					b = c.B
				} else if r1 < 170 {
					if r1 < 128 {
						g = c.R - r2
					} else {
						g = c.R + r2
					}
					r = c.R
					b = c.B
				} else {
					if r1 < 213 {
						b = c.B - r2
					} else {
						b = c.B + r2
					}
					r = c.R
					g = c.G
				}

				return color.NRGBA{r, g, b, c.A}
				})
			isRand = true
		}


		//colorFilter := imaging.New(1000, 1000, color.Gray{} )
		//img = imaging.OverlayCenter(img, colorFilter, 0.5)

	}

	if blur != 0.0 {
		img = imaging.Blur(img, blur)
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

	// yanırlar önbelleğe alınacaksa ve önbelleğe başka bir istek içinden
	// erişilmiyorsa yazma işlemi yapılır. rastgele oluşturulan resimler asla önbelleğe alınmaz
	if cacheResponses && !isRand && !lockRespCache{
		// önbelleğe yazacağız. farklı threadlerden aynı anda yazma
		// işlemi yapmamak için bu kiliti aktifleştirelim
		lockRespCache = true
		respCache[uri] = resp
		log.Printf("writed %s to response cache", uri)
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
