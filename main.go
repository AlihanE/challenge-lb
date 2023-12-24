package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
)

type Balancer interface {
	GetClient() http.Client
}

func main() {
	f, err := os.Open("conf.json")
	if err != nil {
		panic(err)
	}

	b, err := io.ReadAll(f)
	if err != nil {
		panic(err)
	}

	var apis []string
	err = json.Unmarshal(b, &apis)
	if err != nil {
		panic(err)
	}

	clients := []*Client{}
	for _, api := range apis {
		a := NewClient(api)
		a.StartHealthCheck()
		clients = append(clients, a)
	}

	rr := NewRoundRobin(clients)

	e := echo.New()
	e.Any("*", func(c echo.Context) error {
		cli, err := rr.GetClient()
		if err != nil {
			c.Error(err)
			return err
		}

		resp, err := cli.Send(c.Request().Method, c.Request().RequestURI, c.Request().Body)
		if err != nil {
			c.Error(err)
			return err
		}

		return c.String(http.StatusOK, string(resp))
	})
	e.Logger.Fatal(e.Start(":80"))
}

type Client struct {
	baseAddr string
	healthy  bool
}

func NewClient(addr string) *Client {
	return &Client{
		baseAddr: "http://" + addr,
		healthy:  true,
	}
}

func (c *Client) StartHealthCheck() {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			resp, err := http.Get(c.baseAddr + "/health")
			if err != nil {
				fmt.Println("server", c.baseAddr, "StartHealthCheck Do", err)
				c.healthy = false
				continue
			}

			if resp.StatusCode != 200 {
				fmt.Println("server", c.baseAddr, "StartHealthCheck Do", err, "status", resp.StatusCode)
				c.healthy = false
			} else {
				c.healthy = true
			}
		}
	}()
}

func (c *Client) Send(m, uri string, b io.Reader) ([]byte, error) {
	req, err := http.NewRequest(m, c.baseAddr+uri, b)
	if err != nil {
		fmt.Println("server", c.baseAddr, "NewRequest", err)
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("server", c.baseAddr, "Do", err)
		c.healthy = false
		return nil, err
	}

	respB, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("server", c.baseAddr, "ReadAll", err)
		return nil, err
	}

	return respB, nil
}

type RoundRobin struct {
	clients   []*Client
	currentId int
}

func NewRoundRobin(arr []*Client) *RoundRobin {
	return &RoundRobin{
		clients: arr,
	}
}

func (rr *RoundRobin) GetClient() (*Client, error) {
	cli := rr.clients[rr.currentId]
	if !cli.healthy {
		for i := 0; i < len(rr.clients); i++ {
			cli = rr.clients[rr.currentId]
			rr.currentId++
			if cli.healthy {
				if rr.currentId == len(rr.clients) {
					rr.currentId = 0
				}
				return cli, nil
			}
			if rr.currentId == len(rr.clients) {
				rr.currentId = 0
			}
		}
		return nil, errors.New("all clients unhealthy")
	}
	rr.currentId++
	if rr.currentId == len(rr.clients) {
		rr.currentId = 0
	}

	return cli, nil
}
