package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/gocolly/colly/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type Book struct {
	name        string
	announcer   string
	category    string
	requestName string
}

type WriteCounter struct {
	Total   int64
	Current int64
}

type Download struct {
	book  *Book
	key   *string
	title string
}

var wg sync.WaitGroup

func (w *WriteCounter) Write(bytes []byte) (int, error) {
	n := len(bytes)
	w.Current += int64(n)
	w.PrintProgress()
	return n, nil
}

func (w *WriteCounter) PrintProgress() {
	// fmt.Printf("\r%s", strings.Repeat(" ", 35))
	fmt.Printf("\rDownloading... %d ", w.Current)
	if w.Total == w.Current {
		fmt.Printf("\r complete")
	}
}

func main() {
	var url string
	var threadQuantity int
	var start int
	var end int
	flag.StringVar(&url, "u", "", "请求地址")
	flag.IntVar(&threadQuantity, "q", 10, "下载线程的数量")
	flag.IntVar(&start, "s", 0, "下载开始的位置")
	flag.IntVar(&end, "e", 0, "下载结束的位置")
	flag.Parse()
	if len(url) == 0 {
		fmt.Print("请输入Url地址")
		return
	}

	book := &Book{}
	var key = ""
	ch := make(chan *Download, threadQuantity)

	c := colly.NewCollector()

	c.OnHTML(".book01", func(h *colly.HTMLElement) {
		gbkTitle := h.ChildText(".book01>ul>li>:first-child>strong")
		gbkCategory := h.ChildText(".book01>ul>li:nth-child(2)")
		gbkAnnouncer := h.ChildText(".book01>ul>li:nth-child(5)")
		categorySplit := strings.Split(GbkToUtf8(&gbkCategory), "：")
		announcerSplit := strings.Split(GbkToUtf8(&gbkAnnouncer), "：")
		name := GbkToUtf8(&gbkTitle)

		book.name = name
		book.requestName = name
		book.announcer = announcerSplit[1]
		book.category = categorySplit[1]

		dirPath := "./" + book.name
		exists, _ := Exists(dirPath)
		if !exists {
			os.Mkdir(dirPath, os.ModePerm)
		}

		for i := 0; i < cap(ch); i++ {
			go func() {
				for d := range ch {
					download(d.key, d.book, d.title)
					wg.Done()
				}
			}()
		}
	})

	downloadQuantity := 0
	c.OnHTML(".main03>.summary>.list>ul>li", func(h *colly.HTMLElement) {
		gbkTitle := h.ChildText("a")
		title := GbkToUtf8(&gbkTitle)
		url := h.ChildAttr("a", "href")
		// fmt.Printf("%s %s \n", title, url)
		if len(title) > 0 {
			if start <= 0 {
				if end > 0 {
					if downloadQuantity >= end {
						return
					}
					downloadQuantity++
				}
				if len(key) == 0 {
					for {
						k, err := getKey(&url)
						if err == nil {
							fmt.Print("get key complate \n")
							key = k
							break
						}
					}
				}
				if len(key) > 0 {
					wg.Add(1)

					ch <- &Download{
						book:  book,
						key:   &key,
						title: title,
					}
				}
			} else {
				start--
			}
		}
	})

	_ = c.Visit(url)

	c.Wait()
	wg.Wait()
	close(ch)
}

func getKey(url *string) (string, error) {
	// urlSplits := strings.Split(*addr, "/")
	// id := urlSplits[0]

	fmt.Print(*url)
	reqUrl := "https://img.tingchina.com/play/h5_jsonp.asp"
	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		return "", err
	}

	req.AddCookie(&http.Cookie{Name: "tingNewJieshaoren", Value: "0"})
	req.AddCookie(&http.Cookie{Name: "ASPSESSIONIDAESSCTBC", Value: "BNBBHHNCBJOHDLAFMGLJLHIM"})
	req.AddCookie(&http.Cookie{Name: "UM_distinctid", Value: "178cf1350e370-0ceb2f6abec618-5771031-384000-178cf1350e47bc"})
	req.AddCookie(&http.Cookie{Name: "tNew_play_url", Value: "https://www.tingchina.com/yousheng/" + *url})
	req.Header.Add("referer", "https://www.tingchina.com/yousheng/"+*url)
	// req.Header.Add("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.90 Safari/537.36")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	fmt.Print(res.ContentLength)
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	body := string(bytes)
	keyRegexp := regexp.MustCompile("[0-9a-zA-Z]{32}_[0-9]{9}")
	params := keyRegexp.FindStringSubmatch(body)
	if len(params) > 0 {
		return params[0], nil
	}
	return "", errors.New("没有Key")
}

func download(key *string, book *Book, fileName string) {
	fullPath := fmt.Sprintf("./%s/%s", book.name, fileName)
	exists, _ := Exists(fullPath)
	if exists {
		return
	}

	url := fmt.Sprintf("https://t3344.tingchina.com/yousheng/%s/%s/%s?key=%s", book.category, book.requestName, fileName, *key)
	fmt.Printf("%s \n", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Print(err)
		return
	}
	req.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.90 Safari/537.36")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Print(err)
		return
	}
	if res.StatusCode != 200 {
		res.Body.Close()

		book.requestName = book.name + "_" + book.announcer
		download(key, book, fileName)
		return
	}
	defer res.Body.Close()

	file, err := os.Create(fullPath)
	if err != nil {
		fmt.Print(err)
		return
	}
	defer file.Close()

	counter := &WriteCounter{
		Total: res.ContentLength,
	}
	if _, err := io.Copy(file, io.TeeReader(res.Body, counter)); err != nil {
		return
	}

	filenameWithSuffix := path.Base(fullPath)                  //获取文件名带后缀
	fileSuffix := path.Ext(filenameWithSuffix)                 //获取文件后缀
	name := strings.TrimSuffix(filenameWithSuffix, fileSuffix) //获取文件名

	rewriteID3v2(file, &name)

	rewriteID3v1(file, &name)

}

func rewriteID3v2(file *os.File, fileName *string) {
	gbkTitle := Utf8ToGbk(fileName)
	frame := map[string]string{
		"TIT2": gbkTitle,
		"TPE1": "",
		"TALB": "",
		"TCON": "",
		"COMM": "eng\000",
		"TYER": "",
	}

	bytes := make([]byte, 4086)
	index := 0
	for key := range frame {
		keyBytes := []byte(key)
		kLen := len(keyBytes)
		for i := 0; i < kLen; i++ {
			bytes[index+i] = keyBytes[i]
		}
		index += 11
		v, _ := frame[key]
		contentBytes := []byte(v)
		vLen := len(contentBytes)
		bytes[index-4] = byte(vLen + 1)
		for i := 0; i < vLen; i++ {
			bytes[index+i] = contentBytes[i]
		}
		index += vLen
	}

	file.WriteAt(bytes, 10)
}

func rewriteID3v1(file *os.File, fileName *string) {
	bytes := make([]byte, 124)
	gbkTitle := Utf8ToGbk(fileName)
	nameBytes := []byte(gbkTitle)
	n := len(nameBytes)
	if n > 30 {
		n = 30
	}
	for i := 0; i < n; i++ {
		bytes[i] = nameBytes[i]
	}

	fileInfo, _ := file.Stat()
	offset := fileInfo.Size() - 125
	file.WriteAt(bytes, offset)
}

func GbkToUtf8(s *string) string {
	buf := []byte(*s)
	utf8Reader := transform.NewReader(bytes.NewReader(buf), simplifiedchinese.GBK.NewDecoder())
	utf8Bytes, err := ioutil.ReadAll(utf8Reader)
	if err != nil {
		panic(err)
	}
	return string(utf8Bytes)
}

func Utf8ToGbk(s *string) string {
	buf := []byte(*s)
	gbkReader := transform.NewReader(bytes.NewReader(buf), simplifiedchinese.GBK.NewEncoder())
	gbkBytes, err := ioutil.ReadAll(gbkReader)
	if err != nil {
		panic(err)
	}
	return string(gbkBytes)
}

func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
