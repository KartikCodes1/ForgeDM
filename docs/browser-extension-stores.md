# Publishing the ForgeDM browser extension

The extension is published in three stores. Brave installs from the Chrome
Web Store, so the Chrome listing covers Chrome and Brave (and other Chromium
browsers such as Vivaldi).

| Store | Package to upload | Account |
| --- | --- | --- |
| Chrome Web Store (Chrome, Brave) | `ForgeDM-extension-chrome.zip` | Google account, one-time US$5 fee |
| Microsoft Edge Add-ons | `ForgeDM-extension-chrome.zip` (same file) | Free, through Microsoft Partner Center |
| Firefox Add-ons (AMO) | `ForgeDM-extension-firefox.zip` | Free Firefox account |

Both zips are built by CI: open the latest run on GitHub's **Actions** tab and
download the **ForgeDM-windows** artifact. Images for the listings are in
[`docs/store/extension/`](store/extension).

Reviewers need the ForgeDM app to test the extension, so publish a GitHub
release first (push a tag such as `v1.0.0`) and point them to
<https://github.com/KartikCodes1/ForgeDM/releases/latest>.

## Shared listing text

**Name:** ForgeDM Integration (from the manifest)

**Summary** (132 characters max):
Sends downloads, videos and links from your browser to the ForgeDM download
manager app for faster, resumable downloads.

**Description:**

ForgeDM Integration connects your browser to ForgeDM, a free download manager
for Windows. It needs the ForgeDM app:
https://github.com/KartikCodes1/ForgeDM

When you click a download link, the file goes to ForgeDM instead of the
browser's own downloader. ForgeDM splits it into parts that download at the
same time, so big files finish sooner, and it picks up where it left off if the
connection drops or the PC restarts. The extension passes along the site's
cookies and the page you came from, so downloads that need you to be signed in
still work.

What it adds to your browser:

- Downloads go to ForgeDM automatically. Hold Alt while clicking to let the
  browser download a file itself, or turn catching off from the toolbar.
- A "Download this video" button on web video players, with a choice of
  quality for streamed videos. Copy-protected videos can't be saved, and the
  button isn't available on YouTube.
- "Download with ForgeDM" and "Download all links with ForgeDM" in the
  right-click menu.
- A toolbar popup with the current speed and progress.

Privacy: the extension talks only to the ForgeDM app on your own computer
(127.0.0.1). It has no analytics, sends nothing to us, and works only after
you approve the connection in ForgeDM. Privacy policy:
https://github.com/KartikCodes1/ForgeDM/blob/main/PRIVACY.md

**Category:** Productivity (Chrome, Edge) · Download Management (Firefox)

**Privacy policy URL:** <https://github.com/KartikCodes1/ForgeDM/blob/main/PRIVACY.md>

**Homepage / support URL:** <https://github.com/KartikCodes1/ForgeDM> ·
<https://github.com/KartikCodes1/ForgeDM/issues>

**Notes for reviewers:**

> ForgeDM Integration hands downloads to the ForgeDM desktop app (Windows),
> which it reaches at http://127.0.0.1:52771. To test:
> 1. Install ForgeDM from https://github.com/KartikCodes1/ForgeDM/releases/latest
>    and open it.
> 2. Click the ForgeDM toolbar button, click Connect, then click Allow in the
>    ForgeDM window.
> 3. Click any download link (for example a file on python.org/downloads): the
>    download appears in ForgeDM. Hold Alt while clicking to let the browser
>    download it instead.
> 4. On a page with a video, hover the player and click "Download this video".
> The extension sends link addresses, the page address and that site's cookies
> only to the ForgeDM app on the same computer. No remote code is used.

## Chrome Web Store

1. Sign in at the [Chrome Web Store Developer Dashboard](https://chrome.google.com/webstore/devconsole)
   and pay the one-time registration fee.
2. **Add new item** and upload `ForgeDM-extension-chrome.zip`.
3. **Store listing:** paste the description above. Upload:
   - Store icon: `store-icon-128.png`
   - Screenshots (1280×800): `screenshot-1-catch-downloads.png`,
     `screenshot-2-save-videos.png`, `screenshot-3-toolbar.png`
   - Small promo tile (440×280): `promo-small-440x280.png`
   - Marquee promo tile (1400×560, optional): `promo-marquee-1400x560.png`
4. **Privacy practices:**
   - Single purpose: *Hands downloads, links and videos from the browser to the
     ForgeDM download manager app on the user's computer.*
   - Permission justifications:
     - `downloads`: notices when a download starts and hands it to the ForgeDM
       app instead of the browser's downloader (and lets the browser keep it if
       ForgeDM isn't running or the user holds Alt).
     - `contextMenus`: adds "Download with ForgeDM" and "Download all links
       with ForgeDM" to the right-click menu.
     - `storage`: remembers the connection to the ForgeDM app and the user's
       choice to pause catching; keeps the list of videos found on open tabs.
     - `cookies`: reads the cookies of the site a download comes from, so
       ForgeDM can download files that need the user to be signed in. They are
       sent only to the ForgeDM app on the same computer.
     - `webRequest`: reads response headers to notice video streams so the
       video button can offer them. It doesn't block or change any request.
     - Host permission (all sites): downloads and videos can come from any
       site; needed to read that site's cookies, notice its videos, show the
       video button, and reach the ForgeDM app at 127.0.0.1.
   - Remote code: **No**.
   - Data usage: tick **Authentication information** (cookies), **Web history**
     (the page a download came from) and **Website content** (links and video
     addresses). Certify that data isn't sold, isn't used for unrelated
     purposes and isn't used for credit decisions.
5. Paste the reviewer notes under **Test instructions** and submit. Requests
   for all sites usually mean a longer, manual review.

## Microsoft Edge Add-ons

1. Register for free in [Partner Center](https://partner.microsoft.com/dashboard/microsoftedge/overview)
   under the Microsoft Edge program.
2. **Create new extension** and upload `ForgeDM-extension-chrome.zip`.
3. **Properties:** category Productivity; privacy policy URL; website and
   support URLs; "Does your extension access, collect, or transmit personal
   information?" **Yes** (it sends cookies and addresses to the local app).
4. **Store listings (English):** the description above, the 300×300 logo
   `edge-logo-300.png`, the three screenshots, and optionally the 440×280 and
   1400×560 tiles. Search terms: download manager, download accelerator,
   resume downloads, Brave.
5. Paste the reviewer notes under **Notes for certification** and publish.

## Firefox Add-ons

1. Sign in at the [Add-on Developer Hub](https://addons.mozilla.org/developers/)
   and choose **Submit a New Add-on → On this site**.
2. Upload `ForgeDM-extension-firefox.zip`. Select **Firefox** only (not
   Firefox for Android). When asked about source code, answer **No**: the
   files aren't minified or generated.
3. The manifest declares the data the extension sends (browsing activity,
   website activity, website content), which Firefox shows when installing.
4. **Describe add-on:** summary and description above; categories Download
   Management; support site and privacy policy URLs; license of your choice.
   The reviewer notes explain that the free ForgeDM app is needed.
5. Paste the reviewer notes under **Notes to reviewer** and submit.

## After publishing

Send the three store links (or the extension IDs). With them, ForgeDM's
**Connect a browser** dialog can link to the stores instead of developer
mode, and the installer can offer to add the extension to Chrome, Edge and
Brave the way other download managers do.
