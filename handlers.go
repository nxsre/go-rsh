package rsh

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// 404,找不到路径时的处理
func HandleNotFound(c *gin.Context) {
	fmt.Println("请求的地址未找到，uri: ", c.Request.Method, c.Request.RequestURI)
	//fmt.Println("stack: ", string(debug.Stack()))
	NewResult(c).ErrorCode(404, "资源未找到", nil)
	return
}

// 500，内部发生异常时的处理
func Recover(c *gin.Context) {
	defer func() {
		if err := recover(); err != nil {
			//打印错误堆栈信息
			timeStr := time.Now().Format("2006-01-02 15:04:05")
			fmt.Println("当前时间:", timeStr)
			fmt.Println("当前访问path:", c.FullPath())
			fmt.Println("当前完整地址:", c.Request.URL.String())
			fmt.Println("当前协议:", c.Request.Proto)
			fmt.Println("当前get参数:", GetAllGetParams(c))
			fmt.Println("当前post参数:", GetAllPostParams(c))
			fmt.Println("当前访问方法:", c.Request.Method)
			fmt.Println("当前访问Host:", c.Request.Host)
			fmt.Println("当前IP:", c.ClientIP())
			fmt.Println("当前浏览器:", c.Request.UserAgent())
			fmt.Println("发生异常:", err)
			//global.Logger.Errorf("stack: %v",string(debug.Stack()))
			debug.PrintStack()
			//return
			NewResult(c).ErrorCode(500, "服务器内部错误", nil)
		}
	}()
	//继续后续接口调用
	c.Next()
}

// 放回结果
type Result struct {
	Ctx *gin.Context
}

// 生成result
func NewResult(ctx *gin.Context) *Result {
	return &Result{Ctx: ctx}
}

// 成功
func (r *Result) Success(data interface{}) {
	if data == nil {
		data = gin.H{}
	}
	res := ResultCont{}
	res.Status = "success"
	res.Code = 0
	res.Msg = ""
	res.Data = data
	r.Ctx.JSON(http.StatusOK, res)
	r.Ctx.Abort()
}

// 出错,接受code和msg
func (r *Result) ErrorCode(code int, msg string, data interface{}) {
	if data == nil {
		data = gin.H{}
	}
	res := ResultCont{}
	res.Status = "failed"
	res.Code = code
	res.Msg = msg
	res.Data = data
	r.Ctx.JSON(http.StatusOK, res)
	r.Ctx.Abort()
}

// 返回的结果的内容：
type ResultCont struct {
	Status string      `json:"status"` //提示状态
	Code   int         `json:"code"`   //提示代码
	Msg    string      `json:"msg"`    //提示信息
	Data   interface{} `json:"data"`   //出错
}

// 得到所有get参数
func GetAllGetParams(c *gin.Context) string {
	params := c.Request.URL.Query()
	resStr := ""
	// 遍历并打印所有参数及其值
	for key, values := range params {
		for _, value := range values {
			resStr = resStr + "key:" + key + ",value:" + value + "\n"
		}
	}
	return resStr
}

// 得到所有post参数
func GetAllPostParams(c *gin.Context) string {
	c.Request.ParseMultipartForm(32 << 20)
	resStr := ""
	for k, v := range c.Request.PostForm {
		resStr = resStr + "key:" + k + ",value:" + strings.Join(v, ",") + "\n"
	}
	return resStr
}
