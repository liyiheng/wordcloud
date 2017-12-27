package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"

	"wordcloud/embedded"

	"github.com/elazarl/go-bindata-assetfs"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type (
	Text struct {
		Text       string      `json:"text"`
		Size       float64     `json:"size"`
		Color      string      `json:"color"`
		ColorValue color.Color `json:"-"`
	}
	NetMsg struct {
		Err  string `json:"err,omitempty"`
		Data []byte `json:"data,omitempty"`
	}
	NetParams struct {
		Content []*Text `json:"content"`
		Width   int32   `json:"width"`
		Height  int32   `json:"height"`
		Color   string  `json:"color"`
	}
	point struct {
		x int32
		y int32
	}
)

func (msg *NetMsg) SendTo(w http.ResponseWriter) {
	dat, e := json.Marshal(msg)
	if e != nil {
		log.Println(e.Error())
		return
	}
	w.Write(dat)
	if e != nil {
		log.Println(e.Error())
	}
}

// generate 接收客户端请求解析参数并生成结果
func generate(w http.ResponseWriter, req *http.Request) {
	concurrent <- struct{}{}
	defer func() {
		<-concurrent
	}()
	log.Println("start")
	result := &NetMsg{}
	defer req.Body.Close()
	defer result.SendTo(w)
	payload, _ := ioutil.ReadAll(req.Body)

	var params NetParams
	if e := json.Unmarshal(payload, &params); e != nil {
		log.Println(e)
		result.Err = e.Error()
		return
	}

	if params.Height == 0 || params.Width == 0 {
		e := errors.New("width and height cannot be 0")
		log.Println(e)
		result.Err = e.Error()
		return
	}
	bg := image.White
	rgba := image.NewRGBA(image.Rect(0, 0, int(params.Width), int(params.Height)))
	draw.Draw(rgba, rgba.Bounds(), bg, image.ZP, draw.Src)

	ctx := freetype.NewContext()
	ctx.SetSrc(image.Black)
	ctx.SetDst(rgba)
	ctx.SetDPI(defaultDpi)
	ctx.SetClip(rgba.Bounds())
	ctx.SetFont(fnt)
	bgColor := colorSum(bg.C)
	for i, t := range params.Content {
		log.Println(t.Color, t.Text, t.Size)
		size := int(t.Size)
		ctx.SetFontSize(t.Size)
		// color #232323
		if len(t.Color) == 7 {
			r, _ := strconv.ParseInt(t.Color[1:3], 10, 8)
			g, _ := strconv.ParseInt(t.Color[3:5], 10, 8)
			b, _ := strconv.ParseInt(t.Color[5:7], 10, 8)
			t.ColorValue = color.RGBA{A: 255, R: uint8(r), G: uint8(g), B: uint8(b)}
		} else {
			t.ColorValue = color.RGBA{A: 255}
		}

		ctx.SetSrc(image.NewUniform(t.ColorValue))
		txtSize := measure(defaultDpi, t.Size, t.Text, fnt)
		topX, topY := queryIntegralImage(rgba, txtSize.Round(), size, bgColor, qualityNormal)
		if topX < 0 || topY < 0 {
			log.Printf("no room left, %d of %d worlds finished", i, len(params.Content))
			break
		}
		log.Println(topX, topY)
		// baseline start point of the text
		p := freetype.Pt(int(topX), int(topY)+int(t.Size*3/4))
		_, e := ctx.DrawString(t.Text, p)
		if e != nil {
			log.Println(e.Error())
		}
	}
	f, e := os.Create("tmp.png")
	if e != nil {
		return
	}
	png.Encode(f, rgba)
	f.Close()
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	if e := png.Encode(writer, rgba); e != nil {
		log.Println(e)
		result.Err = e.Error()
		return
	}
	if e := writer.Flush(); e != nil {
		log.Println(e)
		result.Err = e.Error()
		return
	}
	result.Data = b.Bytes()
	log.Println("done")

}

// parseFont 用于将字体文件解析为字体
func parseFont(ttfPath string) (*truetype.Font, error) {
	//f, e := os.Open(ttfPath)
	//if e != nil {
	//	return nil, e
	//}
	//defer f.Close()
	dat, e := embedded.Asset(ttfPath)
	if e != nil {
		return nil, e
	}
	fnt, e := freetype.ParseFont(dat)
	if e != nil {
		return nil, e
	}
	return fnt, nil
}

var (
	fnt        *truetype.Font
	concurrent chan struct{}
)

const (
	defaultDpi    = 72
	qualityLow    = 20
	qualityNormal = 10
	qualityHigh   = 5
)

//go:generate go-bindata -o embedded/bindata.go -pkg embedded -nomemcopy asset/...
func main() {
	fontPath := "asset/wqy-microhei.ttc"
	var e error
	fnt, e = parseFont(fontPath)
	if e != nil {
		log.Fatalln(e)
	}
	concurrent = make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	d := flag.Bool("debug", false, "Debug mode use html/js/css files, or use embedded data")
	flag.Parse()
	mux := http.DefaultServeMux
	log.Println("Debug:", *d)
	if *d {
		// 调试模式下，webUI采用外部文件
		mux.Handle("/", http.FileServer(http.Dir(".")))
	} else {
		// 费调试模式下，使用内嵌的数据
		files := &assetfs.AssetFS{
			Asset:     embedded.Asset,
			AssetDir:  embedded.AssetDir,
			AssetInfo: embedded.AssetInfo,
			Prefix:    "",
		}
		mux.Handle("/", http.FileServer(files))

	}
	go func() {
		mux.HandleFunc("/cloud", generate)
		e := http.ListenAndServe(":8765", mux)
		if e != nil {
			log.Println(e)
		}
		wg.Done()
	}()
	e = OpenURLWithBrowser("http://localhost:8765/asset")
	if e != nil {
		log.Println(e)
	}
	wg.Wait()
}

// measure 用于计算文本宽度
func measure(dpi, size float64, txt string, fnt *truetype.Font) fixed.Int26_6 {
	opt := &truetype.Options{
		DPI:  dpi,
		Size: size,
	}
	face := truetype.NewFace(fnt, opt)

	return font.MeasureString(face, txt)
}

// todo
//
// originImg.At(x,y) == dscImg.At(x,y)
//
// colorSame(c1,c2 color.Color) bool{
// 		return 	c1.r == c2.r &&
//				c1.g == c2.g &&
//				c1.b == c2.b &&
//				c1.a == c2.a
// }
func colorSum(p color.Color) uint32 {
	r, g, b, a := p.RGBA()
	return r + g + b + a
}

// queryIntegralImage 查找符合(sizeX, sizeY)的空白区域，并随机取其一
// 返回随机到的空白区域左上角的坐标，(-1,-1)表示未找到符合条件的
func queryIntegralImage(img image.Image, sizeX, sizeY int, bgColor uint32, quality int) (lTopX, lTopY int32) {
	if quality < qualityHigh {
		quality = qualityHigh
	}
	size := img.Bounds().Size()

	foldX := size.X - sizeX
	foldY := size.Y - sizeY
	points := make([]point, 0)
	// count how many possible locations
	for i := 0; i < foldX; i++ {
		for j := 0; j < foldY; j++ {

			// Rectangle:
			//
			// 		i,j			i+sizeX,j
			//
			// 		i,j+sizeY	i+sizeX,j+sizeY
			//
			blank := true
			for x := i + sizeX; x >= i; x -= quality {
				for y := j + sizeY; y >= j; y -= quality {
					if colorSum(img.At(x, y)) != bgColor {
						blank = false
						break
					}
				}
				if !blank {
					break
				}
			}
			if !blank {
				continue
			}
			points = append(points, point{int32(i), int32(j)})
		}
	}
	if len(points) == 0 {
		// no room left
		return -1, -1
	}
	// pick a location at random
	goal := rand.Int31n(int32(len(points)))
	p := points[goal]
	return p.x, p.y
}

var commands = map[string]string{
	"windows": "cmd /c start",
	"darwin":  "open",
	"linux":   "xdg-open",
}

// OpenURLWithBrowser 调用系统浏览器打开指定URI，
// 目前支持Linux、Darwin、Windows三种平台
func OpenURLWithBrowser(uri string) error {
	run, ok := commands[runtime.GOOS]
	if !ok {
		return errors.New(fmt.Sprintf("opening browser on %s unsupported, pls do it manually", runtime.GOOS))
	}
	cmd := exec.Command(run, uri)
	return cmd.Start()
}
