# qbDaemon - Unpacker daemon for qBittorrent

Introduction
------------

A basic daemon for [qBittorrent](https://www.qbittorrent.org) written in [**Go**](https://golang.org) that assists in unpacking downloaded torrents. Unpack downloaded torrents by assigning them a configurable category directly from the standard qBittorrent web interface.

Features
--------

- Unpack all zip and rar archives.
- Set owner and file permissions on downloaded and unpacked torrents.

Requirements
------------

* [**Go**](https://golang.org) (for building the executable, not for running it).
* `unrar` and `unzip` installed and available in the current system path.

Installation
------------

Install **Go** on the system of your choice, then run:

    go get github.com/pelorat/qbdaemon
    go build

To build qbDaemon for Alpine Linux, from a standard libc based Linux distro, you need to install [musl-libc](https://www.musl-libc.org) and tools. On Ubuntu or another `apt` based system run:

    apt install musl musl-dev musl-tools

Build against the `musl-libc` by running:

    CC=$(which musl-gcc) go build --ldflags '-w -linkmode external -extldflags "-static"'

Configuration
-------------

Generate a default (but incomplete) configuration file using the following command line command. qbDaemon uses a `yaml` based configuration file:

    qbtorrent -wconfig=/etc/qbdaemon/qbd.conf

Replace the path with a path of your choice. By default qbTorrent looks in the current directory for `qbd.conf`. Use the `-config` command line parameter to specify an alternate configuration file location when launching the daemon, for example:

    qbtorrent -config=/etc/qbdaemon.qbd.conf

For more command line options, use `-h` or `--help`

Configuration file content:

```
server: 127.0.0.1
port: 80
username: <username>
password: <password>
destpath: /mnt/unpacked
logpath: /var/log/qbdaemon
permissions:
  mode: 0775
  gid: 0
  uid: 0
polling:
  timeout: 5
  delay: 5
workers:
  unpack: 1
  check: 1
categories:
  default: Completed
  error: Error
  no_archive: NoArchive
  unpack_start: Unpack
  unpack_busy: Unpacking
  unpack_done: Unpacked
```

* `server` and `port` of qBittorrent. You obviously need filesystem access to the files that have been downloaded which means you'll probably be running qbDaemon on the same server, hence the default of 127.0.0.1 and port are sensible defaults unless you have changed the port.

* `username` and `password` are optional depending on how you have configured qBittorrent.

* `destpath` is the path were you want the unpacked files to go.

* `logpath` controls the location of the log file. This key is optional and if left out any output will be sent to standard output.

* `permissions` controls the permissions to set on downloaded files and on unpacked files. This section is optional and can be left out if this functionality is unwanted. If left out the unpacked files will have the permissions of the `umask` of the qbDaemon process. qbDaemon obviously need write access to the `destpath`.

* `timeout` controls how long qbdaemon waits (in seconds) for a reply from qBittorrent.

* `delay` controls how often (in seconds) qbdaemon polls qBittorrent.

* `unpack` and `check` controls how many background threads are assigned to each task. The `check` task is a quick task that scans completed torrents for archives, and it also sets the file permissions. The `unpack` task handles unpacking and does the heavy lifting. A value of `1` will run unpacking jobs sequentially which is most likely what you want in order to avoid disk trashing.

* `categories` configures the category keywords used from the qBittorrent web UI for communicating with qbDaemon. The qbDaemon process will attempt to register these categories with qBittorrent automatically when it starts up.

Usage
-----

When qbDaemon detects a completed download, it will enqeue a `check` task to see if the download contains zip or rar archives. If no archives are found, qbdaemon, will assign the torrent the configured `no_archive` category (`NoArchive` by default) letting you know there's nothing in the download to unpack. If archives are found the `default` category will be assigned (`Completed` by default).

To start the unpacking process, right click the torrent in the web UI and assign it the `unpack_start` category (`Unpack` by default). This is the trigger that enqueues an `unpack` task. When unpacking starts the category will change to `unpack_busy` and finally either change to `unpack_done` or `error`.

There's no harm in trying to unpack a torrent which contains no archives, the category will simply be reset to `no_archive` by the `unpack` task. It's also possible to assign the `unpack_start` category to several torrents at once and also to torrents that have not yet finished downloading. Once they are completed the unpacking will start automatically.

The result of the unpacking process is written to `unpack.log` in the destination folder which will have the name of the torrent and will be located in `destpath`.
