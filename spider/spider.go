package spider

import (
	"asmr-downloader/config"
	"asmr-downloader/model"
	"asmr-downloader/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	browser "github.com/EDDYCJY/fake-useragent"
	"github.com/xxjwxc/gowp/workpool"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var ctx = context.Background()

// ASMRClient ASMR 客户端
type ASMRClient struct {
	GlobalConfig  *config.Config
	Authorization string
	WorkerPool    *workpool.WorkPool
}

// 音轨
type track struct {
	Type             string  `json:"type"`
	Title            string  `json:"title"`
	Children         []track `json:"children,omitempty"`
	Hash             string  `json:"hash,omitempty"`
	WorkTitle        string  `json:"workTitle,omitempty"`
	MediaStreamURL   string  `json:"mediaStreamUrl,omitempty"`
	MediaDownloadURL string  `json:"mediaDownloadUrl,omitempty"`
}

// NewASMRClient 初始化ASMR客户端
func NewASMRClient(maxWorker int, globalConfig *config.Config) *ASMRClient {
	return &ASMRClient{
		WorkerPool:   utils.NewWorkerPool(maxWorker),
		GlobalConfig: globalConfig,
	}
}

// Login 登入获取授权信息
func (asmrClient *ASMRClient) Login() error {
	payload, err := json.Marshal(map[string]string{
		"name":     asmrClient.GlobalConfig.Account,
		"password": asmrClient.GlobalConfig.Password,
	})
	if err != nil {
		fmt.Println("登录失败, 配置文件有误。")
		return err
	}
	client := utils.Client.Get().(*http.Client)
	req, _ := http.NewRequest("POST", "https://api.asmr.one/api/auth/me", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://www.asmr.one/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36")
	resp, err := client.Do(req)
	utils.Client.Put(client)
	if err != nil {
		fmt.Println("登录失败, 网络错误。请尝试通过环境变量的方式设置代理。")
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	all, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("登录失败, 读取响应失败。")
		return err
	}
	res := make(map[string]string)
	err = json.Unmarshal(all, &res)
	asmrClient.Authorization = "Bearer " + res["token"]
	return nil
}

func (asmrClient *ASMRClient) GetVoiceTracks(id string) ([]track, error) {
	client := utils.Client.Get().(*http.Client)
	req, _ := http.NewRequest("GET", "https://api.asmr.one/api/tracks/"+id, nil)
	req.Header.Set("Authorization", asmrClient.Authorization)
	req.Header.Set("Referer", "https://www.asmr.one/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36")
	resp, err := client.Do(req)
	utils.Client.Put(client)
	if err != nil {
		fmt.Println("获取音声信息失败:", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	all, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("获取音声信息失败: ", err)
		return nil, err
	}
	res := make([]track, 0)
	err = json.Unmarshal(all, &res)
	return res, nil
}

func (asmrClient *ASMRClient) DownloadItem(id string, subtitleFlag int) {
	rjId := "RJ" + id
	fmt.Println("作品 RJ 号: ", rjId)
	tracks, err := asmrClient.GetVoiceTracks(id)
	if err != nil {
		fmt.Printf("获取作品: %s音轨失败: %s\n", err.Error())
		return
	}
	basePath := config.GetConfig().DownloadDir
	if subtitleFlag == 1 {
		basePath = filepath.Join(basePath, "subtitle")
	} else if subtitleFlag == 0 {
		basePath = filepath.Join(basePath, "nosubtitle")
	}
	itemStorePath := filepath.Join(basePath, "RJ"+id)
	asmrClient.EnsureFileDirsExist(tracks, itemStorePath)

}

func (asmrClient *ASMRClient) EnsureFileDirsExist(tracks []track, basePath string) {
	path := basePath
	_ = os.MkdirAll(path, os.ModePerm)
	for _, t := range tracks {
		if t.Type != "folder" {
			asmrClient.DownloadFile(t.MediaDownloadURL, path, t.Title)
		} else {
			asmrClient.EnsureFileDirsExist(t.Children, fmt.Sprintf("%s/%s", path, t.Title))
		}
	}
}

func (asmrClient *ASMRClient) DownloadFile(url string, dirPath string, fileName string) {
	if runtime.GOOS == "windows" {
		for _, str := range []string{"?", "<", ">", ":", "/", "\\", "*", "|"} {
			fileName = strings.Replace(fileName, str, "_", -1)
		}
	}
	savePath := dirPath + "/" + fileName
	if utils.FileOrDirExists(savePath) {
		fmt.Printf("文件: %s 已存在, 跳过下载...\n", savePath)
		return
	}
	fmt.Println("正在下载 " + savePath)
	_ = utils.NewFileDownloader(url, dirPath, fileName)()

}

// GetPerPageInfo 获取每页的信息
//
//	@Description:
//	@param authorStr 授权字符串
//	@param pageIndex 分页需要
//	@param subtitleFlag 是否选择字幕
//	@return *model.PageResult
//	@return error
func GetPerPageInfo(authorStr string, pageIndex int, subtitleFlag int) (*model.PageResult, error) {
	var seed int = utils.GenerateReqSeed()
	randomUserAgent := browser.Random()
	//log.Printf("Random: %s\n", randomUserAgent)
	//var reqUrl = "https://api.asmr.one/api/works?order=create_date&sort=desc&page=1&seed=" + strconv.Itoa(seed) + "&subtitle=0"
	var reqUrl = ""
	if subtitleFlag == -1 {
		reqUrl = fmt.Sprintf("https://api.asmr.one/api/works?order=id&sort=desc&page=%d&seed=%d", pageIndex, seed)
	} else {
		reqUrl = fmt.Sprintf("https://api.asmr.one/api/works?order=id&sort=desc&page=%d&seed=%d&subtitle=%d", pageIndex, seed, subtitleFlag)
	}
	var resp = new(model.PageResult)
	client := &http.Client{}
	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		// Handle error
		// ignore here
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	//req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "zh,en;q=0.9,zh-TW;q=0.8,zh-CN;q=0.7,ja;q=0.6")
	req.Header.Set("Authorization", authorStr)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Origin", "https://www.asmr.one")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://www.asmr.one/")
	req.Header.Set("Sec-Ch-UA", `"Not?A_Brand";v="8", "Chromium";v="108", "Google Chrome";v="108"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", "macOS")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("User-Agent", randomUserAgent)

	respond, respError := client.Do(req.WithContext(context.Background()))
	utils.Client.Put(client)

	if respError != nil {
		fmt.Println("请求失败: ", respError.Error())
		return nil, respError
	}
	defer func() { _ = respond.Body.Close() }()
	all, err := io.ReadAll(respond.Body)
	if err != nil {
		fmt.Println("获取接口数据失败: ", err)
		return nil, err
	}
	err = json.Unmarshal(all, resp)
	return resp, nil
}

// GetIndexPageInfo
//
// @Description: 获取首页信息
// @param authorStr
// @param subTitleFlag
// @return *model.PageResult
// @return error
func GetIndexPageInfo(authorStr string, subTitleFlag int) (*model.PageResult, error) {
	return GetPerPageInfo(authorStr, 1, subTitleFlag)
}

// GetAllIndexPageInfo
//
//	@Description: 获取所有数据首页信息
//	@param authorStr
//	@return *model.PageResult
//	@return error
func GetAllIndexPageInfo(authorStr string) (*model.PageResult, error) {
	return GetPerPageInfo(authorStr, 1, -1)
}

//func CollectPagesData(reqUrls []string) []model.PageResult {
//	var result []string
//	//执行的 这里要注意  需要指针类型传入  否则会异常
//	wg := &sync.WaitGroup{}
//	//并发控制
//	limiter := make(chan bool, 10)
//	defer close(limiter)
//
//	response := make(chan string, 20)
//	wgResponse := &sync.WaitGroup{}
//	//处理结果 接收结果
//	go func() {
//		wgResponse.Add(1)
//		for rc := range response {
//			result = append(result, rc)
//		}
//		wgResponse.Done()
//	}()
//	//开启协程处理请求
//	for _, url := range urls {
//		//计数器
//		wg.Add(1)
//		//并发控制 10
//		limiter <- true
//		go httpGet(url, response, limiter, wg)
//	}
//	//发送任务
//	wg.Wait()
//	close(response) //关闭 并不影响接收遍历
//	//处理接收结果
//	wgResponse.Wait()
//	return result
//}
