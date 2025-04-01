# Ctrl+C 2 Are.na

Simple `Go` console utility that monitors the Clipboard. Whenever you `Ctrl+C` it sends the copied text to the specified channel in your Are.na profile.

## Things you need to run this:

- `Go`.
- An Are.na account.
- An `ARENA_PERSONAL_ACCESS_TOKEN`
- The slug of the channel you want to feed (as of `https://www.are.na/your-profile/{your-channel}`)

You will need to add those inside a new `.env` file.

Inside the folder execute in the console `go run .` and start collecting!