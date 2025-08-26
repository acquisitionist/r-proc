# R-Proc - Reddit Data File Processor

R-Proc is a command-line tool for processing Reddit data dumps in zstd-compressed NDJSON format. It supports filtering specific subreddit content, efficiently processing large files, and exporting results.

## Features

- Filter Reddit submissions and comments by field values
- Convert Reddit data to CSV format
- Process large zstd-compressed files efficiently
- Support for parallel processing
- Progress tracking and detailed logging
- Filter using exact match, partial match, or regex patterns

## Installation

Clone and build from source:

```bash
git clone https://github.com/acquisitionist/r-proc.git
cd r-proc
go build
```

## Quick Start

### Configuration

R-Proc uses a single `config.ini` file to configure threads, input/output directories, and filters.  

#### Example `config.ini`:

```
threads = 2

[paths]
input = D:\reddit
output = D:\output

[filters]
field = subreddit
values = wallstreetbets, LivestreamFail
file_filter = .*
match_mode = exact
```

### Filtering

#### `field`

Specify which field to filter posts or comments by one of the following available options:

| Field      | Description                         |
|------------|-------------------------------------|
| subreddit  | Filter by the subreddit's name      |
| author     | Filter by the author's username     |
| title      | Filter by the post's title          |
| selftext   | Filter by the post's text content   |
| body       | Filter by the comment's body        |
| domain     | Filter by the domain of linked content |

#### `values`

Comma-separated list of values to match against the chosen `field`. Multiple values are supported.  
The interpretation of these values depend on the selected `match_mode`

#### `file_filter`

Common regex patterns for filtering input filenames.

| Regex      | Description                         |
|------------|-------------------------------------|
| .*         | Match all files in input            |
| ^RS_.*     | Match files starting with "RS_"     |
| ^RC_.*      | match files starting with "RC_"    |

#### `match_mode`

Mode for matching the values in 'values' against the chosen field.

| Mode       | Description                         |
|------------|-------------------------------------|
| exact      | A value must match exactly (case-insensitive)             |
| partial    | A value matches if it appears anywhere in the field      |
| regex      | A each value is treated as a regular expression     |

### Exportation

R-Proc exports filtered Reddit data in NDJSON format. The available fields depend on whether you are processing submissions or comments.

#### Submissions (`RS_*.zst` files)

| Field   | Description                        |
|---------|------------------------------------|
| author  | Username of the post author        |
| title   | Post title                         |
| score   | Post score (upvotes - downvotes)  |
| created | Timestamp when the post was created|
| link    | Link to the post                   |
| text    | Post text / selftext               |
| url     | URL included in the post (if any) |

#### Comments (`RC_*.zst` files)

| Field   | Description                        |
|---------|------------------------------------|
| author  | Username of the comment author     |
| score   | Comment score (upvotes - downvotes)|
| created | Timestamp when the comment was created|
| link    | Link to the parent post or comment |
| body    | Text content of the comment        |

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

[MIT License](LICENSE)
