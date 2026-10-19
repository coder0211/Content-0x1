// Package http is the identity HTTP adapter: /auth/*, /me, /users.
package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves identity endpoints.
type Handler struct {
	Auth    *usecase.Auth
	Users   *usecase.Users
	Cookies httpx.CookieConfig
}

// Routes registers routes on the /api/v1 router.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/auth/login", h.login)
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Post("/auth/logout", h.logout)
		r.Get("/me", h.me)
	})
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermUserManage))
		r.Get("/users", h.listUsers)
		r.Post("/users", h.createUser)
		r.Get("/users/{id}", h.getUser)
		r.Patch("/users/{id}", h.updateUser)
	})
}

// Authenticator adapts Auth to httpx.Authenticator.
type Authenticator struct{ Auth *usecase.Auth }

// Authenticate implements httpx.Authenticator.
func (a Authenticator) Authenticate(ctx context.Context, token string) (kernel.Actor, httpx.Session, error) {
	actor, s, err := a.Auth.Authenticate(ctx, token)
	if err != nil {
		return kernel.Actor{}, httpx.Session{}, err
	}
	return actor, httpx.Session{ID: s.ID, CSRFToken: s.CSRFToken}, nil
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	meta := kernel.RequestMetaFrom(r.Context())
	out, err := h.Auth.Login(r.Context(), usecase.LoginInput{Email: req.Email, Password: req.Password, IP: meta.IP, UserAgent: meta.UserAgent})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.SetSessionCookies(w, h.Cookies, out.Token, out.CSRFToken, out.ExpiresAt)
	httpx.OK(w, r, loginResponse{User: toMe(out.User), CSRFToken: out.CSRFToken, ExpiresAt: out.ExpiresAt})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if s, ok := httpx.SessionFrom(r); ok {
		if err := h.Auth.Logout(r.Context(), s.ID); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	httpx.ClearSessionCookies(w, h.Cookies)
	httpx.NoContent(w)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	m, err := h.Auth.GetMe(r.Context(), httpx.ActorFrom(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, toMe(m))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	res, err := h.Users.List(r.Context(), httpx.ActorFrom(r), usecase.UserFilter{Query: q.Get("q"), Status: q.Get("status")}, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]userResponse, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, toUser(&res.Items[i]))
	}
	httpx.List(w, r, items, res.NextCursor)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, err := h.Users.Create(r.Context(), httpx.ActorFrom(r), usecase.CreateUserInput{
		Email: req.Email, Name: req.Name, RoleCodes: req.RoleCodes, TempPassword: req.TempPassword,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, toUser(u))
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, err := h.Users.Get(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, toUser(u))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req updateUserRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var u *domain.User
	u, err = h.Users.Update(r.Context(), httpx.ActorFrom(r), id, usecase.UpdateUserInput{Name: req.Name, Status: req.Status, RoleCodes: req.RoleCodes})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, toUser(u))
}
