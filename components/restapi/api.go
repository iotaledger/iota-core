package restapi

import (
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/iota-core/pkg/jwt"
	"github.com/iotaledger/iota-core/pkg/restapi"
)

func apiMiddleware() echo.MiddlewareFunc {
	publicRoutesRegEx, err := restapi.CompileRoutesAsRegexes(ParamsRestAPI.PublicRoutes)
	if err != nil {
		Component.LogFatal(err.Error())
	}

	protectedRoutesRegEx, err := restapi.CompileRoutesAsRegexes(ParamsRestAPI.ProtectedRoutes)
	if err != nil {
		Component.LogFatal(err.Error())
	}

	matchPublic := func(c echo.Context) bool {
		loweredPath := strings.ToLower(c.Request().RequestURI)

		for _, reg := range publicRoutesRegEx {
			if reg.MatchString(loweredPath) {
				return true
			}
		}

		return false
	}

	matchExposed := func(c echo.Context) bool {
		loweredPath := strings.ToLower(c.Request().RequestURI)

		for _, reg := range append(publicRoutesRegEx, protectedRoutesRegEx...) {
			if reg.MatchString(loweredPath) {
				return true
			}
		}

		return false
	}

	// configure JWT auth
	salt := ParamsRestAPI.JWTAuth.Salt
	if len(salt) == 0 {
		Component.LogFatalf("'%s' should not be empty", Component.App().Config().GetParameterPath(&(ParamsRestAPI.JWTAuth.Salt)))
	}

	// API tokens do not expire.
	jwtAuth, err = jwt.NewAuth(salt,
		0,
		deps.Host.ID().String(),
		deps.NodePrivateKey,
	)
	if err != nil {
		Component.LogPanicf("JWT auth initialization failed: %w", err)
	}

	jwtAllow := func(c echo.Context, subject string, claims *jwt.AuthClaims) bool {
		// Allow all JWT created for the API if the endpoints are exposed
		if matchExposed(c) {
			return claims.VerifySubject(subject)
		}

		return false
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		// Skip routes matching the publicRoutes
		publicSkipper := func(c echo.Context) bool {
			return matchPublic(c)
		}

		jwtMiddlewareHandler := jwtAuth.Middleware(publicSkipper, jwtAllow)(next)

		return func(c echo.Context) error {
			// Check if the route should be exposed (public or protected)
			if matchExposed(c) {
				// Apply JWT middleware
				return jwtMiddlewareHandler(c)
			}

			return echo.ErrForbidden
		}
	}
}
