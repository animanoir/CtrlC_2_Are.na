# Ctrl+C 2 Are.na

Simple `Go` utility that monitors the Clipboard. Whenever you `Ctrl+C` it sends the copied text to the specified channel in your Are.na profile.

## Things you need to run/develop the source code:

- `Go` (https://go.dev/).
- An Are.na account.
- An `ARENA_PERSONAL_ACCESS_TOKEN` (https://dev.are.na/oauth/applications).
- The slug of the channel you want to feed (as of `https://www.are.na/{your-profile}/{your-channel}`).

Inside the folder execute in the console `go run .` 

### Final executables

Here is the Windows .exe file: https://github.com/animanoir/CtrlC_2_Are.na/releases/tag/release 

I'll add soon the Mac/Linux apps (or if anyone wants to do it feel free).

## Build

`go build` automatically detects your current operating system and architecture to build for that target by default. However, Go also supports cross-compilation, allowing you to build for different platforms by setting the `GOOS` and `GOARCH` environment variables.

For Windows, this command should work: `go build -ldflags="-H=windowsgui" -o ctrl2arena.exe`

For cross-compilation examples:
```bash
# Build for Windows from any OS
GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" -o ctrl2arena.exe

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o ctrl2arena

# Build for macOS
GOOS=darwin GOARCH=amd64 go build -o ctrl2arena
```

You can see all supported target combinations with: `go tool dist list`

## Use case

I like to read and collect information in my Are.na from books and stuff. I also find tedious to copy/paste it each time. So now this tool automatically does it for me, and I can save important notes outside my main computer. This has made my research easier and funnier.
## Collaboration

Please, feel free to fork and enhance the current code so it becomes easier and beautiful to use!

![Are.na logo](https://d2w9rnfcy7mm78.cloudfront.net/9485135/original_10647a43631b7746e4a0821772aefa41.png?1605218631?bc=0)
![Go Gopher in a biplane](https://go.dev/images/gophers/biplane.svg)

---

Built with ❤️ using Go and the Are.na API