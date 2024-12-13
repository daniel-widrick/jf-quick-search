# Quick Search for Jellyfin audio

An ugly as sin proof of concept from a random web request

This simple app is designed to to connect to your local jellyfin instance, pull all audio items from the api, cache them in a local sqlite database, and then serve a web interface with search results at a reasonable speed.

The search algorithm is.... non-elegant. This whole think is a hack.

![jf-quick-search](https://github.com/user-attachments/assets/f3676240-0f7a-4a45-a0f4-1a6a325e8b22)

## Instructions:

1) Log into the jellyfin server Dashboard and create a new api key

2) copy .env.example to .env and change the values inside the value to mathc your environment.

3) run ```go run server.go```

4) navigate to http://localhost:7777/ and type something in the search field

5) click the link to begin playback

Note: New scrapes happen when the application starts.

---

Do not expose to public internet - There is no authentication

This service actually proxies the stream from jellyfin (It was getting late and i didn't want to hack in templates to update the audio source url intot he html).

---

TODO:

1) ui

2) docker

3) autoplay next?

4) ?? 
