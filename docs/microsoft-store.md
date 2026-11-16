# Publishing ForgeDM in the Microsoft Store

The Store version is signed by Microsoft, so Windows installs it without the
"Windows protected your PC" warning. It updates itself through the Store.

The package is built by CI from `build/windows/msix/`. Publishing needs a
free Microsoft Store developer account and one manual submission in Partner
Center; later updates are an upload of the new package.

## 1. Create the developer account (once)

1. Go to <https://storedeveloper.microsoft.com> and choose **Individual
   developer**. It's free; sign in with your Microsoft account.
2. Complete the identity check if Partner Center asks for it.

## 2. Reserve the name and get the package identity (once)

1. In [Partner Center](https://partner.microsoft.com/dashboard) open
   **Apps and games → New product → MSIX or PWA app**.
2. Reserve the name **ForgeDM**. If it's taken, try **ForgeDM Download
   Manager**.
3. Open **Product management → Product identity** and copy these three values
   into `build/windows/msix/store.json`:

   | Partner Center              | store.json             |
   | --------------------------- | ---------------------- |
   | Package/Identity/Name       | `identityName`         |
   | Package/Identity/Publisher  | `publisher`            |
   | Package/Properties/PublisherDisplayName | `publisherDisplayName` |

4. Commit and push. The next CI run builds the Store package: open the run
   on GitHub's **Actions** tab and download the **ForgeDM-store** artifact,
   which contains `ForgeDM-<version>.msix`.

## 3. Submit

In Partner Center, open ForgeDM and click **Start your submission**.

**Pricing and availability:** Free, all markets.

**Properties:**

- Category: **Utilities & tools**
- Privacy policy URL: <https://github.com/KartikCodes1/ForgeDM/blob/main/PRIVACY.md>
- Website: <https://github.com/KartikCodes1/ForgeDM>
- Support contact: <https://github.com/KartikCodes1/ForgeDM/issues>

**Age ratings:** fill in the questionnaire. ForgeDM downloads files from the
internet that the user chooses, and has no chat, user profiles or sharing.

**Packages:** upload `ForgeDM-<version>.msix`. Leave Windows 10/11 Desktop
selected. Partner Center then asks why the app needs the `runFullTrust`
capability; paste:

> ForgeDM is a desktop download manager. It runs as a full-trust desktop app
> so it can save downloads to any folder the user picks, run a local
> (127.0.0.1) connection that its browser extension uses to hand over
> downloads, keep the PC awake or shut it down when downloads finish (if the
> user asks), and scan finished files with Microsoft Defender.

**Store listing (English):** use the text below, and upload the screenshots
in `docs/screenshots/` (1440×900).

**Submission options → Notes for certification:**

> ForgeDM is a download manager. To try it, click "New download", paste a
> direct link to a file (for example the Python installer link from
> python.org/downloads) and click Download. No account or sign-in is needed.
> The browser extension is optional and isn't part of this package.

Click **Submit to the Store**. Microsoft reviews it, usually within a few
days, and emails you the result.

## Updating

1. Raise `info.productVersion` in `wails.json` (for example 1.0.1).
2. Push; download the new `ForgeDM-store` artifact.
3. In Partner Center, start a new submission, replace the package and submit.

To try the Store version on your own PC before submitting, run
`build\windows\msix\build.ps1 -Test` on Windows (it needs the Windows SDK),
trust the test certificate as the script explains, then double-click the
`.msix` it builds.

---

## Store listing text

**Product name:** ForgeDM

**Short description:**
Fast download manager with multi-connection downloads, resume, browser
capture and scheduling.

**Description:**

ForgeDM makes downloads faster and keeps them organized. It splits each file
into parts that download at the same time, so big files finish sooner, and
when one part finishes early it helps with the others. If your connection
drops or the PC restarts, ForgeDM picks up where it left off, and an expired
link can be swapped for a fresh one without starting over.

Start downloads from your browser: with the ForgeDM extension for Chrome,
Edge or Firefox, the files you download go straight to ForgeDM, and a button
on web video players lets you save the video in the quality you choose.

Stay in control: run downloads in queues, schedule them for the night, limit
the speed for everything or a single download, and pause on metered
connections. Save rules put each file in the right folder automatically, and
Site Grabber finds every file on a website so you can pick what to download.

When a download finishes, ForgeDM can check its checksum, scan it with
Microsoft Defender, unpack archives and open the file. The dashboard shows
your speed over time, how much you've downloaded and the time multi-
connection downloading saved you.

ForgeDM has no ads, no accounts and no tracking.

**Features:**

- Multi-connection downloads that re-split work as parts finish
- Pause and resume, even after a restart
- Catches downloads from Chrome, Edge and Firefox with the free extension
- Saves web videos (HLS streams) in the quality you choose
- Queues and schedules, for example downloading overnight
- Speed limits for all downloads, a queue or one download
- Save rules that sort files into folders automatically
- Site Grabber: find and download the files on a website
- Checksum check, Defender scan and archive extraction when done
- Dashboard with live speed graphs and statistics
- Clipboard watching and a floating drop target
- Dark, graphite and light themes

**Keywords:** download manager, download accelerator, downloader, resume
downloads, batch download, download scheduler, site grabber

**Copyright:** Copyright © 2026 ForgeDM
