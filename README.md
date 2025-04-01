# Ctrl+C 2 Are.na

Simple `Go` console utility that monitors the Clipboard. Whenever you `Ctrl+C` it sends the copied text to the specified channel in your Are.na profile.

## Things you need to run this:

- `Go` (https://go.dev/).
- An Are.na account.
- An `ARENA_PERSONAL_ACCESS_TOKEN` (https://dev.are.na/oauth/applications).
- The slug of the channel you want to feed (as of `https://www.are.na/{your-profile}/{your-channel}`).

You will need to add those inside a new `.env` file.

Inside the folder execute in the console `go run .` and start collecting!

## Use case

I like to read and collect information in my Are.na from books and stuff. I also find tedious to copy/paste it each time. So now this tool automatically does it for me, and I can save important notes outside my main computer.


![Are.na logo](https://d2w9rnfcy7mm78.cloudfront.net/9485135/original_10647a43631b7746e4a0821772aefa41.png?1605218631?bc=0)
![Go Gopher in a biplane](https://go.dev/images/gophers/biplane.svg)