Mezeporta Wrapper is a wrapper for Erupe servers. It starts `erupe-ce.exe`/`erupe-ce` and exposes the launcher data used by [Mezeporta Launcher](https://github.com/LilButter/Mezeporta).

###
## Server Setup

> [!IMPORTANT]
> Make sure you are on [Erupe](https://github.com/Mezeporta/Erupe/releases) 9.3.0 or above (9.3b is not supported) Legacy 9.2.1 and 9.2 are supported.

Download the [Wrapper](https://github.com/LilButter/Mezeporta-Wrapper/releases) and place its contents in your Erupe server root beside `erupe-ce.exe`/`erupe-ce` and `config.json`.

Expected server layout:

```text
ServerRoot/
  Mezeporta-ce.exe
  erupe-ce.exe
  config.json
  MezeportaWrapper.json
  Mezeporta.json
  ClientImages/
  GamePatch/
  BinCustom/
```

Start `Mezeporta-ce.exe`.

Do not start `erupe-ce.exe` separately while using the wrapper. The wrapper starts Erupe and proxies the launcher API.

Make sure the Erupe API is enabled in `config.json`.

###
## Wrapper Config

`MezeportaWrapper.json` controls the wrapper's behavior.

| Key | Default | Purpose |
| --- | --- | --- |
| `erupe_version` | `9.3+` | Use `9.3+` for the latest Erupe server release. Use `9.2.1` or `9.2` for Legacy support |
| `9.2ClientMode` | `""` | Client mode for legacy `Erupe9.2` servers. Leave blank unless you are using a legacy `Erupe9.2` server ONLY NOT 9.2.1. |
| `MaxClientPatch` | `1` | Maximum amount of Users that can download the gamepatch at the same time. |
| `SaveCacheFetch` | `true` | Enables savedata cache data for the launcher character book. |
| `MailFetch` | `true` | Enables launcher mail data. |
| `DistributionFetch` | `true` | Enables unclaimed distribution data. |
| `ClientImagesHosting` | `true` | Serves `/ClientImages/...` from the local `ClientImages/` folder. Set to `false` if your images are hosted somewhere else. |
| `MezeportaLogs` | `true` | Enables wrapper logging. |
| `EnableBinCustom` | `false` | Checks `BinCustom/` before falling back to Erupe `bin/`. |

### 
## What It Provides

- Launcher metadata from `Mezeporta.json`.
- Patch hosting from `GamePatch/`.
- Optional ClientImages hosting from `ClientImages/`.
- Savedata cache endpoints.
- Mail endpoints.
- Distribution endpoints.
- Event endpoints.
- Optional `BinCustom/` overrides.

###
## Launcher Metadata

`Mezeporta.json` controls the images displayed on the Launcher for your server.

| Key | Purpose |
| --- | --- |
| `serverTag` | Footer text shown for the server. |
| `banners` | Event / promotion banner list. Each entry supports `src` and optional `link`. |
| `messages` | Announcement and news entries. `kind: 1` is announcements, `kind: 0` is news. |
| `links` | External link buttons. Each entry supports `name`, `link`, and `icon`. |
| `background` | Classic style background image. |
| `button` | Classic style main button image. |
| `cog` | Classic style Cog logo. |
| `capcom` | Classic style Capcom logo. |
| `headers` | Classic style branch header images. |
| `dialog` | Dialog popup image. |
| `server_patch` | Server patch / patching screen image. |
| `ps4` | PS4 style overrides for background, buttons, footer logos, emblem, and headers. |

###
## Changing Images

If `ClientImagesHosting` is enabled, put images under `ClientImages/` and reference them with `/ClientImages/...`.

Example:

```json
{
  "serverTag": "My Server",
  "background": "/ClientImages/launcher/background.jpg",
  "server_patch": "/ClientImages/launcher/ServerPatch.png",
  "banners": [
    {
      "src": "/ClientImages/launcher/banners/BannerWelcome.png",
      "link": "https://example.com"
    }
  ]
}
```

For PS4 style images, use the `ps4` object:

```json
{
  "ps4": {
    "background": "/ClientImages/launcher/ps4/Background.png",
    "button": "/ClientImages/launcher/ps4/Button.png",
    "capcom": "/ClientImages/launcher/ps4/capcom.png",
    "cog": "/ClientImages/launcher/ps4/cog.png",
    "emblem": "/ClientImages/launcher/ps4/Emblem.png",
    "headers": {
      "online": "/ClientImages/launcher/headers/PS4/Online.png",
      "forward": "/ClientImages/launcher/headers/PS4/Forward.png",
      "g": "/ClientImages/launcher/headers/PS4/G.png",
      "z": "/ClientImages/launcher/headers/PS4/Z.png",
      "zz": "/ClientImages/launcher/headers/PS4/ZZ.png"
    }
  }
}
```

If `ClientImagesHosting` is disabled, use full image URLs instead:

```json
{
  "background": "https://example.com/images/background.jpg",
  "banners": [
    {
      "src": "https://example.com/images/banner.png",
      "link": "https://example.com"
    }
  ]
}
```

###
## Patch Files

Patch files are served from `GamePatch/`.

Branch folders:

```text
GamePatch/
  1-Online/
  2-Forward/
  3-G/
  4-Z/
```

Put version-specific patch files under the matching branch and version folder.

###
## BinCustom

`BinCustom/` is only used when `EnableBinCustom` is enabled.

When enabled, the wrapper checks `BinCustom/` before Erupe `bin/` for supported binary files.

###
## Build

Requirements:

- Go version matching `go.mod`.

From `Wrapper/`:

Windows:

```powershell
go build -o Target\Mezeporta-ce.exe
```

Linux native:

```bash
go build -o Target/Mezeporta-ce
```
