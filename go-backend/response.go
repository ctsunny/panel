package main

import "github.com/gin-gonic/gin"

// R is the standard API response format
type R struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Ts   int64       `json:"ts"`
	Data interface{} `json:"data,omitempty"`
}

func Rok(c *gin.Context, data interface{}) {
	c.JSON(200, R{Code: 0, Msg: "操作成功", Ts: nowMs(), Data: data})
}

func RokMsg(c *gin.Context) {
	c.JSON(200, R{Code: 0, Msg: "操作成功", Ts: nowMs()})
}

func Rerr(c *gin.Context, msg string) {
	c.JSON(200, R{Code: -1, Msg: msg, Ts: nowMs()})
}

func RData(data interface{}) R {
	return R{Code: 0, Msg: "操作成功", Ts: nowMs(), Data: data}
}

func RErrR(msg string) R {
	return R{Code: -1, Msg: msg, Ts: nowMs()}
}
