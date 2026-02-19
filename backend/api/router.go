package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"tinyrdm/backend/services"

	"github.com/gin-gonic/gin"
)

// maxRequestBodySize limits request body to 10MB to prevent memory exhaustion
const maxRequestBodySize = 10 << 20 // 10MB

// SetupRouter creates the Gin router with all API routes and static file serving
func SetupRouter(assets embed.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Request body size limit
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodySize)
		c.Next()
	})

	// Security headers
	r.Use(SecurityHeaders())

	// Strict CORS - only allow same-origin requests
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			// Only allow same-origin: compare Origin with Host
			host := getRequestHost(c)
			// Extract host from origin (e.g., "http://localhost:8088" -> "localhost:8088")
			originHost := origin
			if idx := strings.Index(origin, "://"); idx >= 0 {
				originHost = origin[idx+3:]
			}
			// Strip trailing slash
			originHost = strings.TrimRight(originHost, "/")

			if originHost != host {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// CSRF protection for state-changing requests
	r.Use(csrfProtection())

	// Public routes (no auth required)
	registerAuthRoutes(r)
	r.GET("/api/version", func(c *gin.Context) {
		resp := services.Preferences().GetAppVersion()
		c.JSON(200, resp)
	})

	// WebSocket endpoint (auth checked via cookie + origin)
	r.GET("/ws", wsAuthCheck(), Hub().HandleWebSocket)

	// Protected API routes
	api := r.Group("/api")
	api.Use(AuthMiddleware())
	registerConnectionRoutes(api)
	registerBrowserRoutes(api)
	registerCLIRoutes(api)
	registerMonitorRoutes(api)
	registerPubsubRoutes(api)
	registerPreferencesRoutes(api)
	registerSystemRoutes(api)

	// Serve frontend static files from embedded assets
	distFS, err := fs.Sub(assets, "frontend/dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(distFS))
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			f, ferr := http.FS(distFS).Open(path)
			if ferr == nil {
				f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			c.FileFromFS("/", http.FS(distFS))
		})
	}

	return r
}

// getRequestHost returns the effective host, considering reverse proxy headers
func getRequestHost(c *gin.Context) string {
	// Check X-Forwarded-Host first (reverse proxy)
	if fwdHost := c.GetHeader("X-Forwarded-Host"); fwdHost != "" {
		return fwdHost
	}
	return c.Request.Host
}

// csrfProtection validates Origin/Referer for state-changing requests
func csrfProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only check state-changing methods
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		host := getRequestHost(c)
		// Check Origin header first
		origin := c.GetHeader("Origin")
		if origin != "" {
			originHost := origin
			if idx := strings.Index(origin, "://"); idx >= 0 {
				originHost = origin[idx+3:]
			}
			originHost = strings.TrimRight(originHost, "/")
			if originHost != host {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "msg": "cross-origin request blocked"})
				return
			}
			c.Next()
			return
		}

		// Fallback: check Referer
		referer := c.GetHeader("Referer")
		if referer != "" {
			refererHost := referer
			if idx := strings.Index(referer, "://"); idx >= 0 {
				refererHost = referer[idx+3:]
			}
			if slashIdx := strings.Index(refererHost, "/"); slashIdx >= 0 {
				refererHost = refererHost[:slashIdx]
			}
			if refererHost != host {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "msg": "cross-origin request blocked"})
				return
			}
		}

		c.Next()
	}
}

// wsAuthCheck validates auth and origin for WebSocket connections
func wsAuthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Validate Origin header to prevent Cross-Site WebSocket Hijacking
		origin := c.GetHeader("Origin")
		if origin != "" {
			host := getRequestHost(c)
			originHost := origin
			if idx := strings.Index(origin, "://"); idx >= 0 {
				originHost = origin[idx+3:]
			}
			originHost = strings.TrimRight(originHost, "/")
			if originHost != host {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}

		if !IsAuthEnabled() {
			c.Next()
			return
		}
		token, err := c.Cookie("rdm_token")
		if err != nil || !validateToken(token, getClientIP(c)) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
