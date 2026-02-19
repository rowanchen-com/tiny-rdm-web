package api

import (
	"net/http"
	"tinyrdm/backend/services"
	"tinyrdm/backend/types"

	"github.com/gin-gonic/gin"
)

func registerConnectionRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/connection")

	g.GET("/list", func(c *gin.Context) {
		c.JSON(http.StatusOK, services.Connection().ListConnection())
	})

	g.GET("/get", func(c *gin.Context) {
		name := c.Query("name")
		c.JSON(http.StatusOK, services.Connection().GetConnection(name))
	})

	g.POST("/save", func(c *gin.Context) {
		var req struct {
			Name   string                 `json:"name"`
			Param  types.ConnectionConfig `json:"param"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().SaveConnection(req.Name, req.Param))
	})

	g.POST("/save-sorted", func(c *gin.Context) {
		var req struct {
			Conns []types.Connection `json:"conns"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().SaveSortedConnection(req.Conns))
	})

	g.POST("/test", func(c *gin.Context) {
		var param types.ConnectionConfig
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().TestConnection(param))
	})

	g.DELETE("/delete", func(c *gin.Context) {
		name := c.Query("name")
		c.JSON(http.StatusOK, services.Connection().DeleteConnection(name))
	})

	g.POST("/group/create", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().CreateGroup(req.Name))
	})

	g.POST("/group/rename", func(c *gin.Context) {
		var req struct {
			Name    string `json:"name"`
			NewName string `json:"newName"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().RenameGroup(req.Name, req.NewName))
	})

	g.DELETE("/group/delete", func(c *gin.Context) {
		name := c.Query("name")
		includeConn := c.Query("includeConn") == "true"
		c.JSON(http.StatusOK, services.Connection().DeleteGroup(name, includeConn))
	})

	g.POST("/save-last-db", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
			DB   int    `json:"db"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().SaveLastDB(req.Name, req.DB))
	})

	g.POST("/save-refresh-interval", func(c *gin.Context) {
		var req struct {
			Name     string `json:"name"`
			Interval int    `json:"interval"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().SaveRefreshInterval(req.Name, req.Interval))
	})

	g.POST("/export", func(c *gin.Context) {
		c.JSON(http.StatusOK, services.Connection().ExportConnections())
	})

	g.POST("/import", func(c *gin.Context) {
		c.JSON(http.StatusOK, services.Connection().ImportConnections())
	})

	g.POST("/list-sentinel-masters", func(c *gin.Context) {
		var param types.ConnectionConfig
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().ListSentinelMasters(param))
	})

	g.POST("/parse-url", func(c *gin.Context) {
		var req struct {
			URL string `json:"url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Connection().ParseConnectURL(req.URL))
	})
}
