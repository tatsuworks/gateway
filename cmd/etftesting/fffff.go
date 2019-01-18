package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func main() {

	url := "https://discordapp.com/api/v6/users/@me/guilds"

	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Add("cookie", "__cfduid=dcf2814e337cdc38d83ac20040c77ff821546229768")
	req.Header.Add("Authorization", "Bearer VbvaGs4hl60H4boZo6z9bhXTG2LAGs")

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	fmt.Println(res)
	fmt.Println(string(body))

}
