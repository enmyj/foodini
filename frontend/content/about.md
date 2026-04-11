# About Food Tracker

## Basic Details

Food Tracker is a simple calorie and nutrition tracker that's different in two ways:

1. All data is stored in a Google Sheet in your own Google Drive; there is no database. You can view, edit, or delete the Google Sheet at any time. Note: this does result in a bit of latency compared to normal web applications.

2. Gemini Flash (LLM) parses natural-language meal descriptions (or meal photos!) into structured entries with calories and macros. The LLM also provides meal insights and suggestions based on your basic personal details and goals.

## How it works

- Sign in with your Google account.
- Describe your meals in plain English (e.g. "two eggs, toast with butter, and a coffee with oat milk") or take/upload a photo of your meal.
- The LLM parses your meal data into individual entries with estimated nutrition info.
- Review, edit, and confirm — then it's saved to your personal spreadsheet.

# More Details

## Why does this exist?

I needed to go on the FODMAP diet a few years ago. I downloaded a bunch of food tracker apps but found them all pretty fiddly and annoying to use. I ended up using a Google Sheet to track basic meal details and periodically feeding the data into an LLM for insights. With the help of coding LLMs, I figured why not try to (a) put an interface in front of it and (b) use an LLM to facilitate meal entry.

## Technical details

* The frontend uses `svelte`.
* The backend uses `go` with `echo`.
* Uses vanilla Google Oauth for auth for simplicity and also to facilitate working with Google Sheets.  
* Currently deployed to Google Cloud Run
* It has dark mode OK

The code is [open source on GitHub](https://github.com/enmyj/foodini). Feel free to make PRs or issues, fork it, or self-host with your own API key.

## Who am I?

* https://ianmyjer.com
* you can email me if you want: `ian@ianmyjer.com`
