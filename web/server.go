package web

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/ml"
	"NUMParser/releases"
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed public/css public/js public/img public/index.html public/settings.html
var publicFS embed.FS

func subFS(prefix string) http.FileSystem {
	sub, err := fs.Sub(publicFS, prefix)
	if err != nil {
		log.Fatalf("embed sub %q: %v", prefix, err)
	}
	return http.FS(sub)
}

func mustReadEmbed(name string) []byte {
	buf, err := publicFS.ReadFile(name)
	if err != nil {
		log.Fatalf("embed read %q: %v", name, err)
	}
	return buf
}

var (
	route       *gin.Engine
	currentPort string
	httpServer  *http.Server
)

func setupRouter() *gin.Engine {
	gin.DisableConsoleColor()
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.StaticFS("/css", subFS("public/css"))
	r.StaticFS("/img", subFS("public/img"))
	r.StaticFS("/js", subFS("public/js"))
	r.Static("/releases", config.SaveReleasePath)

	indexHTML := mustReadEmbed("public/index.html")
	settingsHTML := mustReadEmbed("public/settings.html")
	htmlType := "text/html; charset=utf-8"
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, htmlType, indexHTML)
	})

	// Protected settings routes
	settingsGroup := r.Group("/", settingsAuthMiddleware())
	settingsGroup.GET("/settings", func(c *gin.Context) {
		c.Data(http.StatusOK, htmlType, settingsHTML)
	})
	settingsGroup.GET("/settings/", func(c *gin.Context) {
		c.Data(http.StatusOK, htmlType, settingsHTML)
	})
	settingsGroup.GET("/api/settings", func(c *gin.Context) {
		settings, err := config.LoadConfig()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"settings":         settings,
			"restart_required": currentPort != "" && settings.Port != currentPort,
		})
	})
	settingsGroup.POST("/api/settings", func(c *gin.Context) {
		var settings config.ConfigParser
		if err := c.ShouldBindJSON(&settings); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid settings payload"})
			return
		}

		saved, err := config.SaveConfig(settings)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ml.Init()

		c.JSON(http.StatusOK, gin.H{
			"settings":         saved,
			"restart_required": currentPort != "" && saved.Port != currentPort,
		})
	})

	settingsGroup.POST("/api/collections/generate", func(c *gin.Context) {
		go func() {
			releases.GetCollections()
			db.SaveAll()
		}()
		c.JSON(http.StatusOK, gin.H{
			"status":  "started",
			"message": "Генерация коллекций запущена в фоновом режиме",
		})
	})

	// http://127.0.0.1:38888/search?query=venom
	r.GET("/search", func(c *gin.Context) {
		if query, ok := c.GetQuery("query"); ok {
			torrs := db.SearchTorr(query)
			host := config.RutorHost()
			out := make([]interface{}, 0, len(torrs))
			for _, t := range torrs {
				cp := *t
				cp.Link = config.JoinRutorLink(host, cp.Link)
				out = append(out, cp)
			}
			c.JSON(200, out)
			return
		}
		c.Status(http.StatusBadRequest)
		return
	})

	return r
}

func settingsAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sec := config.GetSecurityConfig()
		if !sec.EnableSettings {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "settings interface is disabled"})
			return
		}

		if sec.AdminPassword == "" && sec.AccessKey == "" {
			c.Next()
			return
		}

		if sec.AccessKey != "" {
			reqKey := c.Query("key")
			if reqKey == "" {
				reqKey = c.GetHeader("X-Access-Key")
			}
			if reqKey != "" && reqKey == sec.AccessKey {
				c.Next()
				return
			}
		}

		user, pass, ok := c.Request.BasicAuth()
		if ok {
			if sec.AdminPassword != "" && pass == sec.AdminPassword {
				c.Next()
				return
			}
			if sec.AccessKey != "" && (pass == sec.AccessKey || user == sec.AccessKey) {
				c.Next()
				return
			}
		}

		c.Header("WWW-Authenticate", `Basic realm="numParser Settings"`)
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func Start(port string) {
	log.Println("Init web")
	currentPort = port
	route = setupRouter()
	addr := "0.0.0.0:" + port
	httpServer = &http.Server{
		Addr:    addr,
		Handler: route,
	}
	go func() {
		log.Println("Start web server on", addr)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Println("Error start web server on port", port, ":", err)
		}
	}()
}

func Stop(ctx context.Context) error {
	if httpServer != nil {
		log.Println("Stopping web server...")
		return httpServer.Shutdown(ctx)
	}
	return nil
}
