package server

import (
	"context"
	"net/url"

	"github.com/keys-pub/keys"
	"github.com/keys-pub/keys-ext/http/api"
	"github.com/keys-pub/keys/dstore"
	"github.com/keys-pub/keys/http"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

func (s *Server) putFollow(c echo.Context) error {
	s.logger.Infof("Server %s %s", c.Request().Method, c.Request().URL.String())
	ctx := c.Request().Context()

	body, st, err := readBody(c, true, 64*1024)
	if err != nil {
		return s.ErrResponse(c, st, err)
	}

	auth, err := s.auth(c, newAuth("Authorization", "sender", body))
	if err != nil {
		return s.ErrForbidden(c, err)
	}

	recipient, err := keys.ParseID(c.Param("recipient"))
	if err != nil {
		return s.ErrBadRequest(c, errors.Errorf("invalid recipient"))
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return s.ErrBadRequest(c, err)
	}
	token := form.Get("token")
	if token == "" {
		return s.ErrBadRequest(c, errors.Errorf("invalid token"))
	}

	follow := &api.Follow{Sender: auth.KID, Recipient: recipient, Token: token}
	if err := s.fi.Set(ctx, dstore.Path("follows", recipient, "users", auth.KID), dstore.From(follow)); err != nil {
		return s.ErrInternalServer(c, err)
	}

	var out struct{}
	return JSON(c, http.StatusOK, out)
}

func (s *Server) getFollow(c echo.Context) error {
	s.logger.Infof("Server %s %s", c.Request().Method, c.Request().URL.String())
	ctx := c.Request().Context()

	auth, err := s.auth(c, newAuth("Authorization", "recipient", nil))
	if err != nil {
		return s.ErrForbidden(c, err)
	}
	sender, err := keys.ParseID(c.Param("sender"))
	if err != nil {
		return s.ErrBadRequest(c, errors.Errorf("invalid sender"))
	}

	follow, err := s.follow(ctx, sender, auth.KID)
	if err != nil {
		return s.ErrInternalServer(c, err)
	}

	out := api.FollowResponse{Follow: follow}
	return JSON(c, http.StatusOK, out)
}

func (s *Server) follow(ctx context.Context, sender keys.ID, recipient keys.ID) (*api.Follow, error) {
	var follow api.Follow
	ok, err := s.fi.Load(ctx, dstore.Path("follows", recipient, "users", sender), &follow)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &follow, nil
}

func (s *Server) getFollows(c echo.Context) error {
	s.logger.Infof("Server %s %s", c.Request().Method, c.Request().URL.String())
	ctx := c.Request().Context()

	auth, err := s.auth(c, newAuth("Authorization", "recipient", nil))
	if err != nil {
		return s.ErrForbidden(c, err)
	}

	iter, err := s.fi.DocumentIterator(ctx, dstore.Path("follows", auth.KID, "users"))
	if err != nil {
		return s.ErrInternalServer(c, err)
	}
	follows := []*api.Follow{}
	for {
		doc, err := iter.Next()
		if err != nil {
			return s.ErrInternalServer(c, err)
		}
		if doc == nil {
			break
		}
		var follow api.Follow
		if err := doc.To(&follow); err != nil {
			return s.ErrInternalServer(c, err)
		}
		follows = append(follows, &follow)
	}

	out := api.FollowsResponse{Follows: follows}
	return JSON(c, http.StatusOK, out)
}

func (s *Server) deleteFollow(c echo.Context) error {
	s.logger.Infof("Server %s %s", c.Request().Method, c.Request().URL.String())
	ctx := c.Request().Context()

	auth, err := s.auth(c, newAuth("Authorization", "sender", nil))
	if err != nil {
		return s.ErrForbidden(c, err)
	}

	recipient, err := keys.ParseID(c.Param("recipient"))
	if err != nil {
		return s.ErrBadRequest(c, errors.Errorf("invalid recipient"))
	}

	ok, err := s.fi.Delete(ctx, dstore.Path("follows", recipient, "users", auth.KID))
	if err != nil {
		return s.ErrInternalServer(c, err)
	}
	if !ok {
		return s.ErrNotFound(c, errors.Errorf("follow not found"))
	}

	var out struct{}
	return JSON(c, http.StatusOK, out)
}
