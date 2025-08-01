# checkpoint-lite
A minimal checkpoint/restore tool using CRIU and OverlayFS for fast process state management.

## Build

```bash
go build -o checkpoint-lite
```

## CLI Usage

```bash
./checkpoint-lite init <working_directory>
```

```bash
./checkpoint-lite checkpoint <session> <pid> <checkpoint_id>
````

```bash
./checkpoint-lite restore <session> <checkpoint_id>
```

```bash
./checkpoint-lite list <session>
```

```bash
./checkpoint-lite cleanup <session>
```
