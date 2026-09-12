import QtQuick
import Quickshell
import Quickshell.Io
import qs.Ui
import qs.Commons
import "Model.js" as Model

BarWidget {
    id: root
    moduleName: "mus.quran"

    property var quranService: bar && bar.shell ? bar.shell.firstPartyServiceFor("mus.quran") : null

    // The service registers after this widget may instantiate (plugin load
    // order). firstPartyServiceFor is a plain object lookup with no change
    // signal, so keep polling until it lands; the binding also catches later
    // updates once the service is present.
    Timer {
        interval: 200
        repeat: true
        running: !root.quranService
        onTriggered: {
            if (bar && bar.shell) {
                var svc = bar.shell.firstPartyServiceFor("mus.quran");
                if (svc)
                    root.quranService = svc;
            }
        }
    }

    Connections {
        target: root.quranService
        function onOpenTabRequested(tabName) {
            root.settingsOpen = false;
            root.browseExpanded = true;
            root.activeTab = tabName;
            root.popupOpen = true;
        }
    }

    // readonly property string iconGlyph: ""
    readonly property string iconGlyph: ""

    readonly property string barFontFamily: (root.bar && root.bar.fontFamily) ? root.bar.fontFamily : Style.font.family
    readonly property color barUrgent: (root.bar && root.bar.urgent) ? root.bar.urgent : Color.urgent

    // Popup theme roles (bound to the shell Color singleton, never hex).
    readonly property color fg: Color.popups.text
    readonly property color accentC: Color.accent
    readonly property color mutedC: Color.muted

    property bool popupOpen: false
    property string activeTab: "surah" // "surah" | "reciter" | "qurancom" | "bookmarks"
    property string surahQuery: ""
    property string reciterQuery: ""
    property string bookmarkQuery: ""
    property int listCursor: 0

    // Collapsible browse section: expanded by default for discoverability.
    property bool browseExpanded: true
    property bool settingsOpen: false

    // Download prompt state (first-selection full-mushaf prompt)
    property var pendingDownloadReciter: null // { identifier, name, englishName }

    readonly property var currentSurah: quranService ? quranService.currentSurah : null
    readonly property var currentReciter: quranService ? quranService.currentReciter : null
    readonly property var filteredSurahs: Model.filterSurahs(quranService ? quranService.surahs : [], root.surahQuery)
    readonly property var filteredReciters: Model.filterReciters(quranService ? quranService.reciters : [], root.reciterQuery)

    readonly property var quranComMenuItems: {
        var sNum = root.currentSurah ? root.currentSurah.number : 1;
        var sName = root.currentSurah ? (root.currentSurah.transliteration || "Surah") : "Surah";
        var totalV = root.currentSurah ? (root.currentSurah.total_verses || root.currentSurah.totalVerses || 1) : 1;
        var curAyah = Model.estimateAyah(totalV, seekBar.pos, seekBar.dur);
        var pNum = Model.getSurahPage(sNum);
        var jNum = Model.getSurahJuz(sNum);

        return [
            {
                id: "read_ayah",
                title: "Read Ayah " + curAyah + " on Quran.com",
                desc: sName + " (" + sNum + ":" + curAyah + ") with full Mushaf & audio",
                icon: "📖",
                badge: sNum + ":" + curAyah,
                url: "https://quran.com/" + sNum + "/" + curAyah
            },
            {
                id: "tafsir",
                title: "Tafsir & Commentary",
                desc: "Ibn Kathir, Sa'di & Maarif-ul-Quran for " + sName,
                icon: "📜",
                badge: "Tafsir",
                url: "https://quran.com/" + sNum + ":" + curAyah + "/tafsirs"
            },
            {
                id: "word_by_word",
                title: "Word-by-Word Analysis",
                desc: "Grammar, root words & morphology for each ayah",
                icon: "🔍",
                badge: "Grammar",
                url: "https://quran.com/" + sNum + "?translations=131"
            },
            {
                id: "mushaf_page",
                title: "Madinah Mushaf (Page " + pNum + ")",
                desc: "Standard 15-line Madinah Mushaf layout · Juz " + jNum,
                icon: "📄",
                badge: "Page " + pNum,
                url: "https://quran.com/page/" + pNum
            },
            {
                id: "explore_experience",
                title: "Personalized Experience Guide",
                desc: "Reading goals, Uthmani/IndoPak, themes & speeds",
                icon: "✨",
                badge: "Explore",
                url: "https://quran.com/explore/build-your-personalized-quran-experience"
            },
            {
                id: "reflections",
                title: "Quran Reflections",
                desc: "Community reflections and study notes for " + sName,
                icon: "💡",
                badge: "Reflect",
                url: "https://quran.com/" + sNum + "/reflections"
            },
            {
                id: "reciters",
                title: "50+ World Reciters",
                desc: "Explore reciters and styles on Quran.com",
                icon: "📻",
                badge: "Audio",
                url: "https://quran.com/reciters"
            }
        ];
    }

    // Filtering is cheap, but delegate creation is not. Coalesce rapid
    // keystrokes so the virtualized list only receives settled queries.
    Timer {
        id: reciterSearchTimer
        interval: 120
        repeat: false
        onTriggered: {
            if (root.activeTab === "reciter") {
                root.reciterQuery = searchField.text;
                root.listCursor = 0;
            }
        }
    }

    function lang() {
        return quranService ? quranService.language : Model.DEFAULT_LANGUAGE;
    }

    function tr(key) {
        return Model.tr(root.lang(), key);
    }
    function trArgs(key, args) {
        return Model.trArgs(root.lang(), key, args);
    }

    function currentFiltered() {
        if (root.activeTab === "surah")
            return root.filteredSurahs;
        if (root.activeTab === "reciter")
            return root.filteredReciters;
        if (root.activeTab === "bookmarks") {
            var bms = (quranService && quranService.bookmarks) ? quranService.bookmarks : [];
            if (!root.bookmarkQuery || root.bookmarkQuery.trim() === "")
                return bms;
            var q = root.bookmarkQuery.toLowerCase().trim();
            return bms.filter(function (bm) {
                var name = (bm.surahName || "").toLowerCase();
                var rec = (bm.reciter || "").toLowerCase();
                var note = (bm.note || "").toLowerCase();
                var arab = (bm.surahArabic || "").toLowerCase();
                var ayahStr = "ayah " + bm.ayah;
                return name.indexOf(q) !== -1 || rec.indexOf(q) !== -1 || note.indexOf(q) !== -1 || arab.indexOf(q) !== -1 || ayahStr.indexOf(q) !== -1;
            });
        }
        return root.quranComMenuItems;
    }

    readonly property bool opened: root.popupOpen

    function open() {
        root.settingsOpen = false;
        root.popupOpen = true;
    }

    // Contract KeyboardPanel expects on its `owner`
    function close() {
        root.settingsOpen = false;
        root.popupOpen = false;
    }

    function toggle() {
        if (root.popupOpen)
            root.close();
        else
            root.open();
    }

    onPopupOpenChanged: {
        if (root.popupOpen) {
            // Reset any stale search filter so the reciter/surah lists always
            // show on a fresh open.
            root.reciterQuery = "";
            root.surahQuery = "";
            root.bookmarkQuery = "";
            if (quranService)
                quranService.refreshCacheSize();
        }
    }

    function showQuranCom() {
        root.settingsOpen = false;
        root.browseExpanded = true;
        root.activeTab = "qurancom";
        root.popupOpen = true;
    }

    IpcHandler {
        target: "quran-ui"

        function toggle(): string {
            root.broadcast("toggle");
            return "ok";
        }

        function open(): string {
            root.broadcast("open");
            return "ok";
        }

        function openQuranCom(): string {
            root.broadcast("showQuranCom");
            return "ok";
        }

        function close(): string {
            root.broadcast("close");
            return "ok";
        }
    }

    function playSurah(n) {
        if (!quranService)
            return;
        var id = quranService.reciterId || Model.DEFAULT_RECITER;
        // Playing is always streaming-by-default. Downloading is an explicit
        // action on the row icon or in the reciter tab.
        quranService.playSurah(id, n);
        root.close();
    }

    // Reciter selection keeps the popup open and hops to the surah tab; the
    // download prompt (if any) floats above it.
    function pickReciter(id) {
        if (!quranService)
            return;
        quranService.selectReciter(id);
        root.activeTab = "surah";
        root.listCursor = 0;
        if (quranService.shouldPrompt(id))
            root.pendingDownloadReciter = quranService.reciterFor(id) || {
                identifier: id,
                name: "",
                englishName: id
            };
    }

    function declineDownload() {
        if (quranService && root.pendingDownloadReciter) {
            quranService.setReciterStatus(root.pendingDownloadReciter.identifier, "declined");
        }
        root.pendingDownloadReciter = null;
    }

    // j/k + Enter navigation over the visible list.
    function moveCursor(dy) {
        var count = root.currentFiltered().length;
        if (count === 0)
            return;
        root.listCursor = (root.listCursor + dy + count) % count;
    }

    function activateCursor() {
        if (root.activeTab === "surah") {
            var s = root.filteredSurahs[root.listCursor];
            if (s)
                root.playSurah(s.number);
        } else if (root.activeTab === "reciter") {
            var r = root.filteredReciters[root.listCursor];
            if (r)
                root.pickReciter(r.identifier);
        } else if (root.activeTab === "qurancom") {
            var item = root.quranComMenuItems[root.listCursor];
            if (item)
                Qt.openUrlExternally(item.url);
        } else if (root.activeTab === "bookmarks") {
            var bms = root.currentFiltered();
            var bm = bms[root.listCursor];
            if (bm && quranService) {
                var origIdx = quranService.bookmarks ? quranService.bookmarks.indexOf(bm) : -1;
                if (origIdx !== -1)
                    quranService.pickupBookmark(origIdx);
                else
                    quranService.pickupBookmark(root.listCursor);
                root.close();
            }
        }
    }

    // --- bar icon -----------------------------------------------------------

    visible: true
    implicitWidth: button.implicitWidth
    implicitHeight: button.implicitHeight

    BarIconButton {
        id: button
        anchors.fill: parent
        bar: root.bar
        text: ""
        iconComponent: Component {
            Item {
                anchors.fill: parent
                QuranIcon {
                    anchors.centerIn: parent
                    iconSize: button.opticalSize
                    color: button.active && button.useActiveColor ? button.activeColor : button.foreground
                }
            }
        }
        tooltipText: quranService && quranService.hasMedia ? (quranService.surahLabel(quranService.surahNumber) + " — " + quranService.reciterLabel()) : root.tr("tooltip")
        active: root.popupOpen || (quranService ? !!quranService.playing : false)
        onPressed: function (b) {
            if (b === Qt.RightButton || b === Qt.MiddleButton) {
                if (quranService)
                    quranService.playPause();
            } else {
                root.toggle();
            }
        }
        onWheelMoved: function (delta) {
            if (!quranService)
                return;
            if (delta > 0)
                quranService.previous();
            else
                quranService.next();
        }
    }

    // --- popup --------------------------------------------------------------

    KeyboardPanel {
        id: popup
        anchorItem: button
        bar: root.bar
        owner: root
        open: root.popupOpen
        focusTarget: keyCatcher
        padding: Style.space(22)
        contentWidth: popup.fittedContentWidth(Style.space(440))
        contentHeight: popup.fittedContentHeight(root.settingsOpen ? settingsCol.implicitHeight : col.implicitHeight, Style.space(820))

        PanelKeyCatcher {
            id: keyCatcher
            anchors.fill: parent
            blocked: searchField.activeFocus || languageDropdown.popupOpen
            onMoveRequested: function (dx, dy) {
                if (dy !== 0)
                    root.moveCursor(dy);
            }
            onActivateRequested: root.activateCursor()
            onCloseRequested: root.close()

            Column {
                id: col
                width: parent.width
                spacing: 0
                visible: !root.settingsOpen

                // ---- 1. now-playing header ----
                Item {
                    width: parent.width
                    height: Math.max(headerRow.implicitHeight, settingsButton.implicitHeight)
                    implicitHeight: height

                    Row {
                        id: headerRow
                        anchors.left: parent.left
                        anchors.right: qcomButton.left
                        anchors.rightMargin: Style.space(8)
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: Style.space(12)

                        BorderSurface {
                            width: Style.space(56)
                            height: Style.space(56)
                            radius: Style.space(10)
                            color: Util.alpha(root.accentC, 0.12)
                            borderSpec: Border.controlSpec("normal", Util.alpha(root.accentC, 0.35), root.accentC)

                            QuranIcon {
                                anchors.centerIn: parent
                                iconSize: Style.space(34)
                                color: root.accentC
                            }
                        }

                        Column {
                            width: parent.width - Style.space(68)
                            anchors.verticalCenter: parent.verticalCenter
                            spacing: Style.space(4)

                            Text {
                                width: parent.width
                                text: currentSurah ? Model.surahDisplayLabel(currentSurah, root.lang()) : root.tr("noSurahSelected")
                                color: root.fg
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.subtitle
                                font.bold: true
                                elide: Text.ElideRight
                            }

                            Text {
                                width: parent.width
                                text: currentReciter ? Model.reciterDisplayLabel(currentReciter, root.lang()) : "Quran Player"
                                color: root.mutedC
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.bodySmall
                                elide: Text.ElideRight
                            }
                        }
                    }

                    Item {
                        id: qcomButton
                        anchors.right: settingsButton.left
                        anchors.rightMargin: Style.space(2)
                        anchors.top: parent.top
                        width: Style.space(28)
                        height: Style.space(28)

                        Text {
                            anchors.centerIn: parent
                            text: "󰖟"
                            color: (root.activeTab === "qurancom" && root.browseExpanded && !root.settingsOpen) || qcomArea.containsMouse ? root.accentC : root.mutedC
                            font.family: root.barFontFamily
                            font.pixelSize: Style.font.title
                        }

                        MouseArea {
                            id: qcomArea
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            onClicked: {
                                root.settingsOpen = false;
                                root.browseExpanded = true;
                                root.activeTab = "qurancom";
                            }
                        }

                        PanelToolTip {
                            visible: qcomArea.containsMouse
                            text: "Quran.com Menu"
                            fontFamily: root.barFontFamily
                        }
                    }

                    Item {
                        id: settingsButton
                        anchors.right: parent.right
                        anchors.top: parent.top
                        width: Style.space(28)
                        height: Style.space(28)

                        Text {
                            anchors.centerIn: parent
                            text: "󰒓"
                            color: settingsArea.containsMouse ? root.accentC : root.mutedC
                            font.family: root.barFontFamily
                            font.pixelSize: Style.font.title
                        }

                        MouseArea {
                            id: settingsArea
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            onClicked: root.settingsOpen = true
                        }

                        PanelToolTip {
                            visible: settingsArea.containsMouse
                            text: root.tr("settings")
                            fontFamily: root.barFontFamily
                        }
                    }
                }

                Item {
                    width: 1
                    height: Style.space(20)
                    implicitHeight: Style.space(20)
                }

                // ---- 2. seek bar + time labels (below the bar) ----
                Column {
                    width: parent.width
                    spacing: Style.space(8)

                    Item {
                        id: seekBar
                        width: parent.width
                        height: Style.space(20)
                        implicitHeight: height

                        readonly property int dur: quranService && quranService.player ? quranService.player.duration : 0
                        readonly property int pos: quranService && quranService.player ? quranService.player.position : 0
                        readonly property real frac: dur > 0 ? Math.min(1, Math.max(0, pos / dur)) : 0
                        property real dragFrac: -1

                        Rectangle {
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            height: Style.space(2)
                            radius: height / 2
                            color: Util.alpha(root.mutedC, 0.35)
                        }

                        Rectangle {
                            anchors.left: parent.left
                            anchors.verticalCenter: parent.verticalCenter
                            width: parent.width * (seekBar.dragFrac >= 0 ? seekBar.dragFrac : seekBar.frac)
                            height: Style.space(2)
                            radius: height / 2
                            color: root.accentC
                        }

                        Rectangle {
                            x: Math.max(0, Math.min(parent.width - width, parent.width * (seekBar.dragFrac >= 0 ? seekBar.dragFrac : seekBar.frac) - width / 2))
                            y: (parent.height - height) / 2
                            width: Style.space(8)
                            height: Style.space(8)
                            radius: width / 2
                            color: root.accentC
                            visible: seekBar.dur > 0
                        }

                        MouseArea {
                            anchors.fill: parent
                            cursorShape: Qt.PointingHandCursor
                            enabled: seekBar.dur > 0 && quranService && quranService.player && quranService.player.seekable
                            onPressed: function (m) {
                                seekBar.dragFrac = Math.min(1, Math.max(0, m.x / width));
                            }
                            onPositionChanged: function (m) {
                                if (pressed)
                                    seekBar.dragFrac = Math.min(1, Math.max(0, m.x / width));
                            }
                            onReleased: function () {
                                if (quranService && seekBar.dragFrac >= 0)
                                    quranService.seek(Math.round(seekBar.dragFrac * seekBar.dur));
                                seekBar.dragFrac = -1;
                            }
                        }
                    }

                    Item {
                        width: parent.width
                        height: elapsedText.implicitHeight

                        Text {
                            id: elapsedText
                            anchors.left: parent.left
                            text: quranService && quranService.player ? Model.formatTime(quranService.player.position) : "0:00"
                            color: root.mutedC
                            font.family: root.barFontFamily
                            font.pixelSize: Style.font.caption
                        }

                        Text {
                            anchors.right: parent.right
                            text: quranService && quranService.player ? Model.formatTime(quranService.player.duration) : "0:00"
                            color: root.mutedC
                            font.family: root.barFontFamily
                            font.pixelSize: Style.font.caption
                        }
                    }
                }

                Item {
                    width: 1
                    height: Style.space(22)
                    implicitHeight: Style.space(22)
                }

                // ---- 3. transport row ----
                Row {
                    anchors.horizontalCenter: parent.horizontalCenter
                    spacing: Style.space(22)

                    Item {
                        width: Style.space(38)
                        height: Style.space(46)

                        Button {
                            anchors.verticalCenter: parent.verticalCenter
                            width: Style.space(38)
                            implicitHeight: Style.space(38)
                            horizontalPadding: 0
                            verticalPadding: 0
                            iconText: quranService && quranService.playbackMode === Model.MODE_CONTINUE ? "󰐌" : quranService && quranService.playbackMode === Model.MODE_REPEAT_ONE ? "󰑘" : quranService && quranService.playbackMode === Model.MODE_REPEAT_ALL ? "󰑖" : "󰐍"
                            foreground: root.fg
                            tooltipText: Model.modeLabel(root.lang(), quranService ? quranService.playbackMode : Model.MODE_SINGLE)
                            onClicked: if (quranService)
                                quranService.cycleMode()
                        }
                    }

                    Item {
                        width: Style.space(38)
                        height: Style.space(46)

                        Button {
                            anchors.verticalCenter: parent.verticalCenter
                            width: Style.space(38)
                            implicitHeight: Style.space(38)
                            horizontalPadding: 0
                            verticalPadding: 0
                            iconText: "󰒮"
                            foreground: root.fg
                            onClicked: if (quranService)
                                quranService.previous()
                        }
                    }

                    Item {
                        width: Style.space(46)
                        height: Style.space(46)
                        implicitHeight: Style.space(46)

                        Rectangle {
                            anchors.fill: parent
                            radius: height / 2
                            color: Util.alpha(root.accentC, playArea.containsMouse ? 0.22 : 0.12)
                            border.color: Util.alpha(root.accentC, playArea.containsMouse ? 0.8 : 0.45)
                            border.width: 1.5
                        }

                        Text {
                            anchors.centerIn: parent
                            text: quranService && quranService.isPlaying ? "󰏤" : "󰐊"
                            color: root.fg
                            font.family: root.barFontFamily
                            font.pixelSize: Style.font.iconLarge
                        }

                        MouseArea {
                            id: playArea
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            onClicked: if (quranService)
                                quranService.playPause()
                        }
                    }

                    Item {
                        width: Style.space(38)
                        height: Style.space(46)

                        Button {
                            anchors.verticalCenter: parent.verticalCenter
                            width: Style.space(38)
                            implicitHeight: Style.space(38)
                            horizontalPadding: 0
                            verticalPadding: 0
                            iconText: "󰒭"
                            foreground: root.fg
                            onClicked: if (quranService)
                                quranService.next()
                        }
                    }

                    Item {
                        width: Style.space(38)
                        height: Style.space(46)

                        Button {
                            anchors.verticalCenter: parent.verticalCenter
                            width: Style.space(38)
                            implicitHeight: Style.space(38)
                            horizontalPadding: 0
                            verticalPadding: 0
                            iconText: "󰓛"
                            foreground: root.mutedC
                            onClicked: if (quranService)
                                quranService.stopPlayback()
                        }
                    }
                }

                Item {
                    width: 1
                    height: Style.space(20)
                    implicitHeight: Style.space(20)
                }

                // ---- 3.5. Reader Location Card (Where the reader is) ----
                BorderSurface {
                    id: readerCard
                    width: parent.width
                    height: readerCol.implicitHeight + Style.space(24)
                    radius: Style.space(10)
                    color: Util.alpha(root.accentC, 0.08)
                    borderSpec: Border.controlSpec("normal", Util.alpha(root.accentC, 0.35), root.accentC)

                    Column {
                        id: readerCol
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.margins: Style.space(12)
                        spacing: Style.space(8)

                        Item {
                            width: parent.width
                            height: Math.max(ayahRow.implicitHeight, progressBadge.implicitHeight)

                            Row {
                                id: ayahRow
                                anchors.left: parent.left
                                anchors.verticalCenter: parent.verticalCenter
                                spacing: Style.space(8)

                                Text {
                                    text: "📖"
                                    font.pixelSize: Style.font.bodySmall
                                }

                                Text {
                                    readonly property int totalV: root.currentSurah ? (root.currentSurah.total_verses || root.currentSurah.totalVerses || 1) : 1
                                    readonly property int curAyah: Model.estimateAyah(totalV, seekBar.pos, seekBar.dur)
                                    text: root.currentSurah ? ("Ayah " + curAyah + " of " + totalV) : "Quran Reader"
                                    color: root.fg
                                    font.family: root.barFontFamily
                                    font.pixelSize: Style.font.bodySmall
                                    font.bold: true
                                }
                            }

                            Rectangle {
                                id: progressBadge
                                anchors.right: parent.right
                                anchors.verticalCenter: parent.verticalCenter
                                width: progressBadgeText.implicitWidth + Style.space(14)
                                height: progressBadgeText.implicitHeight + Style.space(6)
                                radius: height / 2
                                color: Util.alpha(root.accentC, 0.22)

                                Text {
                                    id: progressBadgeText
                                    anchors.centerIn: parent
                                    text: Math.round(seekBar.frac * 100) + "%"
                                    color: root.accentC
                                    font.family: root.barFontFamily
                                    font.pixelSize: Style.font.caption
                                    font.bold: true
                                }
                            }
                        }

                        Row {
                            width: parent.width
                            spacing: Style.space(6)

                            readonly property int sNum: root.currentSurah ? root.currentSurah.number : 1
                            readonly property int jNum: Model.getSurahJuz(sNum)
                            readonly property int pNum: Model.getSurahPage(sNum)

                            Text {
                                text: "Juz " + parent.jNum + " · Hizb " + ((parent.jNum * 2) - 1) + " · Page " + parent.pNum + (root.currentSurah && root.currentSurah.type ? (" · " + root.currentSurah.type.toUpperCase()) : "")
                                color: root.mutedC
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.caption
                            }
                        }

                        Rectangle {
                            width: parent.width
                            height: Style.spaceReal(0.5)
                            color: Util.alpha(root.accentC, 0.25)
                        }

                        Row {
                            width: parent.width
                            spacing: Style.space(8)

                            readonly property int totalV: root.currentSurah ? (root.currentSurah.total_verses || root.currentSurah.totalVerses || 1) : 1
                            readonly property int curAyah: Model.estimateAyah(totalV, seekBar.pos, seekBar.dur)
                            readonly property int sNum: root.currentSurah ? root.currentSurah.number : 1

                            BorderSurface {
                                height: Style.space(26)
                                width: quranLinkRow.implicitWidth + Style.space(16)
                                radius: height / 2
                                color: Util.alpha(root.accentC, quranLinkMouse.containsMouse ? 0.3 : 0.14)
                                borderSpec: Border.controlSpec("normal", Util.alpha(root.accentC, 0.35), root.accentC)

                                Row {
                                    id: quranLinkRow
                                    anchors.centerIn: parent
                                    spacing: Style.space(5)
                                    Text {
                                        text: "󰖟"
                                        color: root.accentC
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        anchors.verticalCenter: parent.verticalCenter
                                    }
                                    Text {
                                        text: parent.parent.parent.sNum + ":" + parent.parent.parent.curAyah + " ↗"
                                        color: root.fg
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        font.bold: true
                                        anchors.verticalCenter: parent.verticalCenter
                                    }
                                }

                                MouseArea {
                                    id: quranLinkMouse
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: {
                                        var s = root.currentSurah ? root.currentSurah.number : 1;
                                        var v = root.currentSurah ? (root.currentSurah.total_verses || root.currentSurah.totalVerses || 1) : 1;
                                        var a = Model.estimateAyah(v, seekBar.pos, seekBar.dur);
                                        Qt.openUrlExternally("https://quran.com/" + s + "/" + a);
                                    }
                                }
                            }

                            BorderSurface {
                                height: Style.space(26)
                                width: tafsirLinkText.implicitWidth + Style.space(16)
                                radius: height / 2
                                color: Util.alpha(root.fg, tafsirLinkMouse.containsMouse ? 0.15 : 0.06)
                                borderSpec: Border.controlSpec("normal", Util.alpha(root.fg, 0.15), root.fg)

                                Text {
                                    id: tafsirLinkText
                                    anchors.centerIn: parent
                                    text: "Tafsir ↗"
                                    color: root.fg
                                    font.family: root.barFontFamily
                                    font.pixelSize: Style.font.caption
                                }

                                MouseArea {
                                    id: tafsirLinkMouse
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: {
                                        var s = root.currentSurah ? root.currentSurah.number : 1;
                                        var v = root.currentSurah ? (root.currentSurah.total_verses || root.currentSurah.totalVerses || 1) : 1;
                                        var a = Model.estimateAyah(v, seekBar.pos, seekBar.dur);
                                        Qt.openUrlExternally("https://quran.com/" + s + ":" + a + "/tafsirs");
                                    }
                                }
                            }

                            BorderSurface {
                                visible: !quranService || !quranService.bookmarks || quranService.bookmarks.length === 0
                                height: Style.space(26)
                                width: qcomMenuLinkRow.implicitWidth + Style.space(16)
                                radius: height / 2
                                color: Util.alpha(root.accentC, qcomMenuLinkMouse.containsMouse || (root.activeTab === "qurancom" && root.browseExpanded) ? 0.25 : 0.08)
                                borderSpec: Border.controlSpec("normal", Util.alpha(root.accentC, 0.35), root.accentC)

                                Row {
                                    id: qcomMenuLinkRow
                                    anchors.centerIn: parent
                                    spacing: Style.space(4)

                                    Text {
                                        text: "Quran.com"
                                        color: root.accentC
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        font.bold: true
                                        anchors.verticalCenter: parent.verticalCenter
                                    }

                                    Text {
                                        text: "▾"
                                        color: root.accentC
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        anchors.verticalCenter: parent.verticalCenter
                                    }
                                }

                                MouseArea {
                                    id: qcomMenuLinkMouse
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: {
                                        root.settingsOpen = false;
                                        root.browseExpanded = true;
                                        root.activeTab = "qurancom";
                                    }
                                }
                            }

                            BorderSurface {
                                height: Style.space(26)
                                width: bmActionRow.implicitWidth + Style.space(16)
                                radius: height / 2
                                color: Util.alpha(root.accentC, bookmarkConfirmTimer.running ? 0.35 : (bmActionMouse.containsMouse ? 0.25 : 0.12))
                                borderSpec: Border.controlSpec("normal", Util.alpha(root.accentC, 0.4), root.accentC)

                                Row {
                                    id: bmActionRow
                                    anchors.centerIn: parent
                                    spacing: Style.space(5)

                                    Text {
                                        text: bookmarkConfirmTimer.running ? "✓" : "🔖"
                                        color: root.accentC
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        anchors.verticalCenter: parent.verticalCenter
                                    }

                                    Text {
                                        text: bookmarkConfirmTimer.running ? "Saved!" : "Bookmark"
                                        color: root.fg
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        font.bold: true
                                        anchors.verticalCenter: parent.verticalCenter
                                    }
                                }

                                MouseArea {
                                    id: bmActionMouse
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: {
                                        if (quranService) {
                                            quranService.addBookmark();
                                            bookmarkConfirmTimer.restart();
                                        }
                                    }
                                }
                            }

                            Timer {
                                id: bookmarkConfirmTimer
                                interval: 1800
                                repeat: false
                            }

                            BorderSurface {
                                visible: quranService && quranService.bookmarks && quranService.bookmarks.length > 0
                                height: Style.space(26)
                                width: pickupTopRow.implicitWidth + Style.space(16)
                                radius: height / 2
                                color: Util.alpha(root.accentC, pickupTopMouse.containsMouse ? 0.32 : 0.16)
                                borderSpec: Border.controlSpec("normal", Util.alpha(root.accentC, 0.45), root.accentC)

                                Row {
                                    id: pickupTopRow
                                    anchors.centerIn: parent
                                    spacing: Style.space(5)

                                    Text {
                                        text: "󰐊"
                                        color: root.accentC
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        anchors.verticalCenter: parent.verticalCenter
                                    }

                                    Text {
                                        text: "Pick Up"
                                        color: root.accentC
                                        font.family: root.barFontFamily
                                        font.pixelSize: Style.font.caption
                                        font.bold: true
                                        anchors.verticalCenter: parent.verticalCenter
                                    }
                                }

                                MouseArea {
                                    id: pickupTopMouse
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: {
                                        if (quranService) {
                                            quranService.pickupBookmark(0);
                                            root.close();
                                        }
                                    }
                                }

                                PanelToolTip {
                                    visible: pickupTopMouse.containsMouse
                                    text: (quranService && quranService.bookmarks && quranService.bookmarks.length > 0) ? ("Pick up latest: " + quranService.bookmarks[0].surahName + " (Ayah " + quranService.bookmarks[0].ayah + ")") : "Pick up last bookmark"
                                    fontFamily: root.barFontFamily
                                }
                            }
                        }
                    }
                }

                Item {
                    width: 1
                    height: Style.space(18)
                    implicitHeight: Style.space(18)
                }

                // ---- 4. browse toggle row ----
                MouseArea {
                    width: parent.width
                    height: browseToggle.implicitHeight
                    implicitHeight: height
                    cursorShape: Qt.PointingHandCursor
                    onClicked: root.browseExpanded = !root.browseExpanded

                    Column {
                        id: browseToggle
                        width: parent.width
                        spacing: Style.space(12)

                        Rectangle {
                            width: parent.width
                            height: Style.spaceReal(0.5)
                            color: Util.alpha(root.mutedC, 0.35)
                        }

                        Row {
                            anchors.horizontalCenter: parent.horizontalCenter
                            spacing: Style.space(6)

                            Text {
                                text: root.tr("browse")
                                color: root.mutedC
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.bodySmall
                                font.weight: Font.Medium
                                anchors.verticalCenter: parent.verticalCenter
                            }

                            Text {
                                text: root.browseExpanded ? "" : ""
                                color: root.mutedC
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.caption
                                anchors.verticalCenter: parent.verticalCenter
                            }
                        }
                    }
                }

                // ---- 5. collapsible browse section ----
                Item {
                    id: browseBlock
                    width: parent.width
                    clip: true
                    height: root.browseExpanded ? Style.space(20) + browseContent.implicitHeight : 0
                    implicitHeight: height

                    Behavior on height {
                        NumberAnimation {
                            duration: 150
                            easing.type: Easing.OutCubic
                        }
                    }

                    Column {
                        id: browseContent
                        width: parent.width
                        spacing: 0

                        Item {
                            width: 1
                            height: Style.space(20)
                            implicitHeight: Style.space(20)
                        }

                        // ---- error / loading ----
                        Column {
                            width: parent.width
                            spacing: Style.space(4)
                            visible: quranService && (quranService.errorMessage !== "" || quranService.recitersLoading)

                            Text {
                                width: parent.width
                                visible: quranService && quranService.errorMessage !== ""
                                text: quranService ? quranService.errorMessage : ""
                                color: root.barUrgent
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.bodySmall
                                wrapMode: Text.WordWrap
                            }

                            Row {
                                visible: quranService && quranService.errorMessage !== ""
                                spacing: Style.space(6)

                                Button {
                                    text: root.tr("retry")
                                    foreground: root.fg
                                    horizontalPadding: Style.spacing.controlPaddingX
                                    verticalPadding: Style.spacing.controlPaddingY
                                    onClicked: {
                                        if (!quranService)
                                            return;
                                        if (quranService.lastDownload && quranService.errorMessage === root.tr("downloadFailed"))
                                            quranService.retryDownload();
                                        else
                                            quranService.retry();
                                    }
                                }
                            }

                            Text {
                                width: parent.width
                                visible: quranService && quranService.recitersLoading
                                text: root.tr("loadingReciters")
                                color: root.mutedC
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.caption
                            }
                        }

                        // ---- tabs ----
                        Row {
                            width: parent.width
                            spacing: Style.space(8)

                            Button {
                                width: (parent.width - parent.spacing * 3) / 4
                                implicitHeight: Style.space(34)
                                text: root.tr("tabSurah")
                                selected: root.activeTab === "surah"
                                bordered: true
                                foreground: root.fg
                                fontSize: Style.font.caption
                                horizontalPadding: Style.space(4)
                                verticalPadding: Style.space(6)
                                onClicked: {
                                    root.activeTab = "surah";
                                    root.listCursor = 0;
                                }
                            }

                            Button {
                                width: (parent.width - parent.spacing * 3) / 4
                                implicitHeight: Style.space(34)
                                text: (quranService && quranService.bookmarks && quranService.bookmarks.length > 0) ? ("Marks (" + quranService.bookmarks.length + ")") : "Marks"
                                selected: root.activeTab === "bookmarks"
                                bordered: true
                                foreground: root.fg
                                fontSize: Style.font.caption
                                horizontalPadding: Style.space(4)
                                verticalPadding: Style.space(6)
                                onClicked: {
                                    root.activeTab = "bookmarks";
                                    root.listCursor = 0;
                                }
                            }

                            Button {
                                width: (parent.width - parent.spacing * 3) / 4
                                implicitHeight: Style.space(34)
                                text: root.tr("tabReciter")
                                selected: root.activeTab === "reciter"
                                bordered: true
                                foreground: root.fg
                                fontSize: Style.font.caption
                                horizontalPadding: Style.space(4)
                                verticalPadding: Style.space(6)
                                onClicked: {
                                    root.activeTab = "reciter";
                                    root.listCursor = 0;
                                }
                            }

                            Button {
                                width: (parent.width - parent.spacing * 3) / 4
                                implicitHeight: Style.space(34)
                                text: root.tr("tabQuranCom")
                                selected: root.activeTab === "qurancom"
                                bordered: true
                                foreground: root.fg
                                fontSize: Style.font.caption
                                horizontalPadding: Style.space(4)
                                verticalPadding: Style.space(6)
                                onClicked: {
                                    root.activeTab = "qurancom";
                                    root.listCursor = 0;
                                }
                            }
                        }

                        Item {
                            width: 1
                            height: Style.space(12)
                            implicitHeight: Style.space(12)
                        }

                        // ---- search + list ----
                        Column {
                            width: parent.width
                            spacing: Style.space(8)

                            TextField {
                                id: searchField
                                visible: root.activeTab !== "qurancom" && (root.activeTab !== "bookmarks" || (quranService && quranService.bookmarks && quranService.bookmarks.length > 0))
                                width: parent.width
                                verticalPadding: Style.space(8)
                                horizontalPadding: Style.space(12)
                                placeholderText: root.activeTab === "surah" ? root.tr("searchSurah") : (root.activeTab === "reciter" ? root.tr("searchReciter") : "Search bookmarks...")
                                foreground: root.fg
                                font.family: root.barFontFamily
                                text: root.activeTab === "surah" ? root.surahQuery : (root.activeTab === "reciter" ? root.reciterQuery : root.bookmarkQuery)
                                onTextChanged: {
                                    if (root.activeTab === "surah") {
                                        root.surahQuery = text;
                                        root.listCursor = 0;
                                    } else if (root.activeTab === "reciter") {
                                        reciterSearchTimer.restart();
                                    } else {
                                        root.bookmarkQuery = text;
                                        root.listCursor = 0;
                                    }
                                }
                            }

                            ListView {
                                id: listView
                                visible: root.activeTab !== "qurancom" && root.activeTab !== "bookmarks"
                                width: parent.width
                                height: Math.min(Style.space(300), Math.max(Style.space(48), contentHeight))
                                implicitHeight: height
                                clip: true
                                spacing: Style.space(4)
                                boundsBehavior: Flickable.StopAtBounds
                                model: root.activeTab === "surah" ? root.filteredSurahs : (root.activeTab === "reciter" ? root.filteredReciters : [])

                                delegate: BorderSurface {
                                    id: rowDelegate
                                    required property var modelData
                                    required property int index

                                    readonly property var surah: root.activeTab === "surah" ? modelData : null
                                    readonly property var reciter: root.activeTab === "reciter" ? modelData : null
                                    readonly property bool selected: root.activeTab === "surah" ? (quranService && surah && surah.number === quranService.surahNumber) : (quranService && reciter && reciter.identifier === quranService.reciterId)
                                    readonly property bool hoveredCursor: index === root.listCursor
                                    readonly property int downloadRevision: quranService ? quranService.downloadRevision : 0
                                    readonly property bool fullDownloaded: {
                                        downloadRevision;
                                        return root.activeTab === "reciter" && quranService && reciter ? quranService.isMushafDownloaded(reciter.identifier) : false;
                                    }
                                    readonly property bool surahDownloaded: {
                                        downloadRevision;
                                        return root.activeTab === "surah" && quranService && surah ? quranService.isSurahDownloaded(quranService.reciterId, surah.number) : false;
                                    }
                                    readonly property bool isDownloading: root.activeTab === "surah" ? (quranService && surah ? quranService.isSurahDownloading(quranService.reciterId, surah.number) : false) : (quranService && reciter ? quranService.isReciterDownloading(reciter.identifier) : false)

                                    readonly property color rowTitle: selected ? Qt.lighter(root.accentC, 1.2) : root.fg
                                    readonly property color rowSubtitle: selected ? Util.alpha(root.accentC, 0.75) : root.mutedC
                                    readonly property color actionColor: surahDownloaded || fullDownloaded || isDownloading ? root.accentC : root.mutedC

                                    width: listView.width
                                    height: Style.space(48)
                                    radius: Style.space(8)
                                    color: selected ? Util.alpha(root.accentC, 0.16) : (hoveredCursor ? Util.alpha(root.fg, 0.07) : "transparent")
                                    borderSpec: selected ? Border.flat(root.accentC, "0 0 0 2") : Border.none()

                                    // Row click handler is declared BEFORE the row content so the
                                    // download action (below) sits above it in z-order; otherwise the
                                    // full-size MouseArea swallows the icon clicks.
                                    MouseArea {
                                        anchors.fill: parent
                                        hoverEnabled: true
                                        cursorShape: Qt.PointingHandCursor
                                        onClicked: {
                                            if (root.activeTab === "surah")
                                                root.playSurah(surah.number);
                                            else
                                                root.pickReciter(reciter.identifier);
                                        }
                                    }

                                    Row {
                                        id: rowInner
                                        anchors.left: parent.left
                                        anchors.right: parent.right
                                        anchors.verticalCenter: parent.verticalCenter
                                        anchors.leftMargin: parent.borderLeft + Style.space(10)
                                        anchors.rightMargin: parent.borderRight + Style.space(10)
                                        spacing: Style.space(10)

                                        Text {
                                            text: root.activeTab === "surah" ? (selected ? "󰕾" : (surah.number + "")) : (selected ? "󰐊" : "󰐍")
                                            color: selected ? root.accentC : root.mutedC
                                            font.family: root.barFontFamily
                                            font.pixelSize: root.activeTab === "surah" && selected ? Style.space(14) : Style.font.bodySmall
                                            width: Style.space(26)
                                            horizontalAlignment: Text.AlignHCenter
                                            anchors.verticalCenter: parent.verticalCenter
                                        }

                                        Column {
                                            width: parent.width - Style.space(72)
                                            spacing: Style.space(2)
                                            anchors.verticalCenter: parent.verticalCenter

                                            Text {
                                                text: root.activeTab === "surah" ? Model.surahListLabel(surah, root.lang()) : Model.reciterDisplayLabel(reciter, root.lang())
                                                color: rowDelegate.rowTitle
                                                font.family: root.barFontFamily
                                                font.pixelSize: Style.font.bodySmall
                                                font.weight: selected ? Font.Medium : Font.Normal
                                                elide: Text.ElideRight
                                                width: parent.width
                                            }

                                            Text {
                                                text: root.activeTab === "surah" && surah ? (surah.transliteration + " · " + surah.type) : (reciter && reciter.name !== "" ? reciter.name : "")
                                                color: rowDelegate.rowSubtitle
                                                font.family: root.barFontFamily
                                                font.pixelSize: Style.font.caption
                                                elide: Text.ElideRight
                                                width: parent.width
                                                visible: text !== ""
                                            }
                                        }

                                        // Download status icon: outline (muted) → web-style spinner
                                        // (accent) → check-circle (accent). Dedicated 26x26 hit area with its own
                                        // MouseArea so the click downloads instead of falling through to
                                        // the row (select/play).
                                        Item {
                                            anchors.verticalCenter: parent.verticalCenter
                                            width: Style.space(26)
                                            height: Style.space(26)
                                            implicitHeight: height
                                            z: 10

                                            Rectangle {
                                                anchors.fill: parent
                                                z: 10
                                                radius: Style.spacing.labelGap
                                                color: downloadActionArea.containsMouse ? Util.alpha(root.accentC, 0.14) : "transparent"
                                                border.color: downloadActionArea.containsMouse ? Util.alpha(root.accentC, 0.55) : "transparent"
                                                border.width: 1
                                            }

                                            Text {
                                                anchors.centerIn: parent
                                                visible: !isDownloading
                                                text: root.activeTab === "surah" ? (surahDownloaded ? "󰗠" : "󰇚") : (fullDownloaded ? "󰗠" : "󰇚")
                                                color: rowDelegate.actionColor
                                                font.family: root.barFontFamily
                                                font.pixelSize: Style.font.bodySmall
                                            }

                                            Item {
                                                anchors.centerIn: parent
                                                visible: isDownloading
                                                width: Style.space(18)
                                                height: Style.space(18)

                                                // Eight dots with a staggered opacity wave. This is an
                                                // indeterminate spinner, rather than a glyph rotating in
                                                // place or a fabricated percentage.
                                                Repeater {
                                                    model: 8

                                                    Rectangle {
                                                        required property int index
                                                        width: Style.space(3)
                                                        height: Style.space(5)
                                                        radius: width / 2
                                                        color: root.accentC
                                                        x: (parent.width - width) / 2 + Math.sin(index * Math.PI / 4) * Style.space(5.5)
                                                        y: (parent.height - height) / 2 - Math.cos(index * Math.PI / 4) * Style.space(5.5)
                                                        rotation: index * 45

                                                        SequentialAnimation on opacity {
                                                            loops: Animation.Infinite
                                                            running: isDownloading
                                                            PauseAnimation { duration: index * 90 }
                                                            NumberAnimation { to: 1.0; duration: 140 }
                                                            PauseAnimation { duration: 450 }
                                                            NumberAnimation { to: 0.25; duration: 140 }
                                                            PauseAnimation { duration: (7 - index) * 90 }
                                                        }
                                                    }
                                                }
                                            }

                                            MouseArea {
                                                id: downloadActionArea
                                                anchors.fill: parent
                                                z: 20
                                                hoverEnabled: true
                                                cursorShape: Qt.PointingHandCursor
                                                onClicked: function (mouse) {
                                                    mouse.accepted = true;
                                                    if (!quranService || isDownloading)
                                                        return;
                                                    if (root.activeTab === "surah")
                                                        Quickshell.execDetached(["omarchy-shell", "quran", "download", quranService.reciterId, String(surah.number)]);
                                                    else
                                                        // IPC requires the surah argument; 0 means the full
                                                        // reciter set and is handled as a mushaf download.
                                                        Quickshell.execDetached(["omarchy-shell", "quran", "download", reciter.identifier, "0"]);
                                                }
                                            }

                                            PanelToolTip {
                                                visible: downloadActionArea.containsMouse
                                                text: root.activeTab === "surah" ? (isDownloading ? root.trArgs("downloading", [Model.reciterDisplayLabel(root.currentReciter, root.lang())]) : (surahDownloaded ? root.tr("downloaded") : root.tr("download"))) : (isDownloading ? root.trArgs("downloading", [Model.reciterDisplayLabel(reciter, root.lang())]) : (fullDownloaded ? root.tr("downloaded") : root.tr("download")))
                                                fontFamily: root.barFontFamily
                                            }
                                        }
                                    }
                                }
                            }

                            ListView {
                                id: quranComListView
                                visible: root.activeTab === "qurancom"
                                width: parent.width
                                height: Style.space(330)
                                implicitHeight: height
                                clip: true
                                spacing: Style.space(8)
                                boundsBehavior: Flickable.StopAtBounds
                                model: root.quranComMenuItems

                                delegate: BorderSurface {
                                    id: qcomDelegate
                                    required property var modelData
                                    required property int index

                                    readonly property bool hoveredCursor: index === root.listCursor || qcomMouse.containsMouse

                                    width: quranComListView.width
                                    height: Style.space(58)
                                    radius: Style.space(10)
                                    color: hoveredCursor ? Util.alpha(root.accentC, 0.16) : Util.alpha(root.fg, 0.04)
                                    borderSpec: hoveredCursor ? Border.flat(root.accentC, 1) : Border.flat(Util.alpha(root.fg, 0.1), 1)

                                    MouseArea {
                                        id: qcomMouse
                                        anchors.fill: parent
                                        hoverEnabled: true
                                        cursorShape: Qt.PointingHandCursor
                                        onClicked: {
                                            Qt.openUrlExternally(modelData.url);
                                        }
                                    }

                                    BorderSurface {
                                        id: qcomIconBox
                                        anchors.left: parent.left
                                        anchors.leftMargin: Style.space(12)
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: Style.space(36)
                                        height: Style.space(36)
                                        radius: Style.space(8)
                                        color: Util.alpha(root.accentC, 0.15)

                                        Text {
                                            anchors.centerIn: parent
                                            text: modelData.icon
                                            font.pixelSize: Style.space(16)
                                        }
                                    }

                                    BorderSurface {
                                        id: qcomBadge
                                        anchors.right: parent.right
                                        anchors.rightMargin: Style.space(12)
                                        anchors.verticalCenter: parent.verticalCenter
                                        height: Style.space(24)
                                        width: qcomBadgeText.implicitWidth + Style.space(12)
                                        radius: height / 2
                                        color: Util.alpha(root.accentC, 0.2)

                                        Text {
                                            id: qcomBadgeText
                                            anchors.centerIn: parent
                                            text: modelData.badge + " ↗"
                                            color: root.accentC
                                            font.family: root.barFontFamily
                                            font.pixelSize: Style.font.caption
                                            font.bold: true
                                        }
                                    }

                                    Column {
                                        anchors.left: qcomIconBox.right
                                        anchors.leftMargin: Style.space(12)
                                        anchors.right: qcomBadge.left
                                        anchors.rightMargin: Style.space(12)
                                        anchors.verticalCenter: parent.verticalCenter
                                        spacing: Style.space(3)

                                        Text {
                                            width: parent.width
                                            text: modelData.title
                                            color: qcomDelegate.hoveredCursor ? root.accentC : root.fg
                                            font.family: root.barFontFamily
                                            font.pixelSize: Style.font.bodySmall
                                            font.bold: true
                                            elide: Text.ElideRight
                                        }

                                        Text {
                                            width: parent.width
                                            text: modelData.desc
                                            color: root.mutedC
                                            font.family: root.barFontFamily
                                            font.pixelSize: Style.font.caption
                                            elide: Text.ElideRight
                                        }
                                    }
                                }
                            }

                            ListView {
                                id: bookmarksListView
                                visible: root.activeTab === "bookmarks" && root.currentFiltered().length > 0
                                width: parent.width
                                height: Math.min(Style.space(320), Math.max(Style.space(64), contentHeight))
                                implicitHeight: height
                                clip: true
                                spacing: Style.space(8)
                                boundsBehavior: Flickable.StopAtBounds
                                model: root.currentFiltered()

                                delegate: BorderSurface {
                                    id: bmDelegate
                                    required property var modelData
                                    required property int index

                                    readonly property bool hoveredCursor: index === root.listCursor || bmMouse.containsMouse

                                    width: bookmarksListView.width
                                    height: Style.space(64)
                                    radius: Style.space(10)
                                    color: hoveredCursor ? Util.alpha(root.accentC, 0.16) : Util.alpha(root.fg, 0.04)
                                    borderSpec: hoveredCursor ? Border.flat(root.accentC, 1) : Border.flat(Util.alpha(root.fg, 0.1), 1)

                                    MouseArea {
                                        id: bmMouse
                                        anchors.fill: parent
                                        hoverEnabled: true
                                        cursorShape: Qt.PointingHandCursor
                                        onClicked: {
                                            if (quranService) {
                                                var origIdx = quranService.bookmarks ? quranService.bookmarks.indexOf(modelData) : -1;
                                                if (origIdx !== -1)
                                                    quranService.pickupBookmark(origIdx);
                                                else
                                                    quranService.pickupBookmark(index);
                                                root.close();
                                            }
                                        }
                                    }

                                    // Left Icon Tile
                                    BorderSurface {
                                        id: bmIconBox
                                        anchors.left: parent.left
                                        anchors.leftMargin: Style.space(12)
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: Style.space(40)
                                        height: Style.space(40)
                                        radius: Style.space(8)
                                        color: Util.alpha(root.accentC, 0.15)

                                        QuranIcon {
                                            anchors.centerIn: parent
                                            iconSize: Style.space(24)
                                            color: root.accentC
                                        }
                                    }

                                    // Right Action Buttons: Pick Up + Delete
                                    Row {
                                        id: bmActionsRow
                                        anchors.right: parent.right
                                        anchors.rightMargin: Style.space(12)
                                        anchors.verticalCenter: parent.verticalCenter
                                        spacing: Style.space(8)

                                        // Pick Up button
                                        BorderSurface {
                                            height: Style.space(30)
                                            width: bmPickupRow.implicitWidth + Style.space(18)
                                            radius: Style.space(6)
                                            color: Util.alpha(root.accentC, bmPickupMouse.containsMouse ? 0.35 : 0.2)
                                            borderSpec: Border.flat(root.accentC, 1)

                                            Row {
                                                id: bmPickupRow
                                                anchors.centerIn: parent
                                                spacing: Style.space(4)

                                                Text {
                                                    text: "󰐊"
                                                    color: root.accentC
                                                    font.pixelSize: Style.space(11)
                                                    anchors.verticalCenter: parent.verticalCenter
                                                }

                                                Text {
                                                    text: "Pick Up"
                                                    color: root.accentC
                                                    font.family: root.barFontFamily
                                                    font.pixelSize: Style.font.caption
                                                    font.bold: true
                                                    anchors.verticalCenter: parent.verticalCenter
                                                }
                                            }

                                            MouseArea {
                                                id: bmPickupMouse
                                                anchors.fill: parent
                                                hoverEnabled: true
                                                cursorShape: Qt.PointingHandCursor
                                                onClicked: {
                                                    if (quranService) {
                                                        var origIdx = quranService.bookmarks ? quranService.bookmarks.indexOf(modelData) : -1;
                                                        if (origIdx !== -1)
                                                            quranService.pickupBookmark(origIdx);
                                                        else
                                                            quranService.pickupBookmark(index);
                                                        root.close();
                                                    }
                                                }
                                            }
                                        }

                                        // Remove button
                                        BorderSurface {
                                            height: Style.space(30)
                                            width: Style.space(30)
                                            radius: Style.space(6)
                                            color: bmRemoveMouse.containsMouse ? Util.alpha(root.barUrgent, 0.2) : "transparent"
                                            borderSpec: bmRemoveMouse.containsMouse ? Border.flat(root.barUrgent, 1) : Border.none()

                                            Text {
                                                anchors.centerIn: parent
                                                text: "󰆴"
                                                color: bmRemoveMouse.containsMouse ? root.barUrgent : root.mutedC
                                                font.family: root.barFontFamily
                                                font.pixelSize: Style.font.caption
                                            }

                                            MouseArea {
                                                id: bmRemoveMouse
                                                anchors.fill: parent
                                                hoverEnabled: true
                                                cursorShape: Qt.PointingHandCursor
                                                onClicked: {
                                                    if (quranService) {
                                                        var origIdx = quranService.bookmarks ? quranService.bookmarks.indexOf(modelData) : -1;
                                                        if (origIdx !== -1)
                                                            quranService.removeBookmark(origIdx);
                                                        else
                                                            quranService.removeBookmark(index);
                                                    }
                                                }
                                            }

                                            PanelToolTip {
                                                visible: bmRemoveMouse.containsMouse
                                                text: "Delete bookmark"
                                                fontFamily: root.barFontFamily
                                            }
                                        }
                                    }

                                    // Center Content Column
                                    Column {
                                        anchors.left: bmIconBox.right
                                        anchors.leftMargin: Style.space(12)
                                        anchors.right: bmActionsRow.left
                                        anchors.rightMargin: Style.space(12)
                                        anchors.verticalCenter: parent.verticalCenter
                                        spacing: Style.space(4)

                                        Row {
                                            width: parent.width
                                            spacing: Style.space(6)

                                            Text {
                                                text: (modelData.surahName || ("Surah " + modelData.surah)) + (modelData.surahArabic ? (" · " + modelData.surahArabic) : "")
                                                color: bmDelegate.hoveredCursor ? root.accentC : root.fg
                                                font.family: root.barFontFamily
                                                font.pixelSize: Style.font.bodySmall
                                                font.bold: true
                                                elide: Text.ElideRight
                                                anchors.verticalCenter: parent.verticalCenter
                                            }

                                            BorderSurface {
                                                height: Style.space(18)
                                                width: ayahTag.implicitWidth + Style.space(10)
                                                radius: Style.space(4)
                                                color: Util.alpha(root.accentC, 0.15)
                                                anchors.verticalCenter: parent.verticalCenter

                                                Text {
                                                    id: ayahTag
                                                    anchors.centerIn: parent
                                                    text: "Ayah " + (modelData.ayah || 1)
                                                    color: root.accentC
                                                    font.family: root.barFontFamily
                                                    font.pixelSize: Style.space(10)
                                                    font.bold: true
                                                }
                                            }
                                        }

                                        Text {
                                            width: parent.width
                                            text: (modelData.reciter || "Mishary Alafasi") + " · " + (modelData.timestamp_ms ? Model.formatTime(modelData.timestamp_ms) : "0:00") + (modelData.juz ? (" · Juz " + modelData.juz) : "")
                                            color: root.mutedC
                                            font.family: root.barFontFamily
                                            font.pixelSize: Style.font.caption
                                            elide: Text.ElideRight
                                        }
                                    }

                                    PanelToolTip {
                                        visible: bmMouse.containsMouse && !bmPickupMouse.containsMouse && !bmRemoveMouse.containsMouse
                                        text: (modelData.surahName || ("Surah " + modelData.surah)) + " (Ayah " + (modelData.ayah || 1) + ") · " + (modelData.reciter || "") + " · " + (modelData.timestamp_ms ? Model.formatTime(modelData.timestamp_ms) : "0:00") + (modelData.juz ? (" · Juz " + modelData.juz + " · Page " + modelData.page) : "")
                                        fontFamily: root.barFontFamily
                                    }
                                }
                            }

                            // Empty state for bookmarks when no bookmarks are saved yet
                            Column {
                                width: parent.width
                                visible: root.activeTab === "bookmarks" && (!quranService || !quranService.bookmarks || quranService.bookmarks.length === 0)
                                spacing: Style.space(8)
                                topPadding: Style.space(20)
                                bottomPadding: Style.space(20)

                                Text {
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: "🔖"
                                    font.pixelSize: Style.space(28)
                                }

                                Text {
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: "No Bookmarks Yet"
                                    color: root.fg
                                    font.family: root.barFontFamily
                                    font.pixelSize: Style.font.bodySmall
                                    font.bold: true
                                }

                                Text {
                                    width: parent.width - Style.space(40)
                                    anchors.horizontalCenter: parent.horizontalCenter
                                    text: "Click the 🔖 Bookmark button above while listening to save your spot in the Quran."
                                    color: root.mutedC
                                    font.family: root.barFontFamily
                                    font.pixelSize: Style.font.caption
                                    horizontalAlignment: Text.AlignHCenter
                                    wrapMode: Text.WordWrap
                                }
                            }

                            // Empty state is outside the virtualized list so it
                            // does not become a delegate or affect scrolling.
                            Text {
                                width: parent.width
                                visible: (root.activeTab === "surah" || root.activeTab === "reciter" || (root.activeTab === "bookmarks" && quranService && quranService.bookmarks && quranService.bookmarks.length > 0)) && root.currentFiltered().length === 0
                                text: root.trArgs("noResults", [root.activeTab === "surah" ? root.surahQuery : (root.activeTab === "reciter" ? root.reciterQuery : root.bookmarkQuery)])
                                color: root.mutedC
                                font.family: root.barFontFamily
                                font.pixelSize: Style.font.bodySmall
                                wrapMode: Text.WordWrap
                                horizontalAlignment: Text.AlignHCenter
                            }
                        }
                    }
                }
            }

            Column {
                id: settingsCol
                width: parent.width
                spacing: Style.space(16)
                visible: root.settingsOpen

                Row {
                    width: parent.width
                    spacing: Style.space(8)

                    Button {
                        width: Style.space(28)
                        implicitHeight: Style.space(28)
                        horizontalPadding: 0
                        verticalPadding: 0
                        iconText: "󰁍"
                        foreground: root.mutedC
                        onClicked: root.settingsOpen = false
                    }

                    Text {
                        text: root.tr("settings")
                        color: root.fg
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.title
                        font.bold: true
                        anchors.verticalCenter: parent.verticalCenter
                    }
                }

                Column {
                    width: parent.width
                    spacing: Style.space(6)

                    Text {
                        text: root.tr("language")
                        color: root.mutedC
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.caption
                    }

                    SearchableDropdown {
                        id: languageDropdown
                        width: parent.width
                        value: root.lang()
                        options: Model.LANGUAGES
                        showLabel: false
                        foreground: root.fg
                        placeholderText: root.tr("language")
                        onChanged: function (v) {
                            if (quranService)
                                quranService.setLanguage(v);
                        }
                    }
                }

                BorderSurface {
                    id: clearCacheButton
                    width: parent.width
                    height: Style.space(36)
                    radius: Style.spacing.labelGap
                    color: Util.alpha(root.barUrgent, clearCacheArea.containsMouse ? 0.22 : 0.12)
                    borderSpec: Border.flat(root.barUrgent, 1)

                    Text {
                        anchors.left: parent.left
                        anchors.leftMargin: Style.space(12)
                        anchors.verticalCenter: parent.verticalCenter
                        text: "󰆴"
                        color: root.barUrgent
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.bodySmall
                    }

                    Text {
                        anchors.left: parent.left
                        anchors.leftMargin: Style.space(36)
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.tr("clearCache")
                        color: root.barUrgent
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.bodySmall
                    }

                    Text {
                        anchors.right: parent.right
                        anchors.rightMargin: Style.space(12)
                        anchors.verticalCenter: parent.verticalCenter
                        text: quranService ? root.trArgs("cacheSize", [Model.formatSize(quranService.proxySizeBytes)]) : ""
                        color: root.mutedC
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.caption
                    }

                    MouseArea {
                        id: clearCacheArea
                        anchors.fill: parent
                        hoverEnabled: true
                        cursorShape: Qt.PointingHandCursor
                        onClicked: if (quranService)
                            quranService.clearCache()
                    }
                }
            }
        }
    }

    // ---- download prompt overlay ----
    KeyboardPanel {
        id: downloadPopup
        anchorItem: button
        bar: root.bar
        owner: root
        open: root.pendingDownloadReciter !== null
        padding: Style.space(20)
        contentWidth: downloadPopup.fittedContentWidth(Style.space(300))
        contentHeight: downloadPopup.fittedContentHeight(downloadCol.implicitHeight)

        Column {
            id: downloadCol
            anchors.fill: parent
            spacing: Style.space(10)

            Row {
                width: parent.width
                spacing: Style.space(10)

                BorderSurface {
                    width: Style.space(48)
                    height: Style.space(48)
                    radius: Style.spacing.labelGap
                    color: Style.normalFillFor(root.fg, root.accentC)
                    borderSpec: Border.controlSpec("normal", root.fg, root.accentC)

                    Text {
                        anchors.centerIn: parent
                        text: root.iconGlyph
                        color: root.fg
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.title
                    }
                }

                Column {
                    width: parent.width - Style.space(58)
                    spacing: Style.space(3)
                    anchors.verticalCenter: parent.verticalCenter

                    Text {
                        width: parent.width
                        text: root.tr("download")
                        color: root.fg
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.subtitle
                        font.bold: true
                    }

                    Text {
                        width: parent.width
                        text: root.pendingDownloadReciter ? Model.reciterDisplayLabel(root.pendingDownloadReciter, root.lang()) : ""
                        color: root.accentC
                        font.family: root.barFontFamily
                        font.pixelSize: Style.font.bodySmall
                        elide: Text.ElideRight
                    }
                }
            }

            Text {
                width: parent.width
                text: {
                    if (!quranService)
                        return "";
                    if (quranService.downloading)
                        return root.trArgs("downloading", [root.pendingDownloadReciter ? Model.reciterDisplayLabel(root.pendingDownloadReciter, root.lang()) : ""]);
                    var id = root.pendingDownloadReciter ? root.pendingDownloadReciter.identifier : "";
                    if (id) {
                        var missing = quranService.missingCount(id);
                        if (missing > 0 && missing < 114)
                            return root.trArgs("downloadRemaining", [String(missing)]);
                    }
                    return root.tr("downloadDesc");
                }
                color: root.mutedC
                font.family: root.barFontFamily
                font.pixelSize: Style.font.bodySmall
                wrapMode: Text.WordWrap
            }

            // progress bar
            Rectangle {
                width: parent.width
                height: Style.spacing.controlHeight
                radius: Style.cornerRadius
                color: Style.controlFill(false, false, root.fg, root.accentC)
                visible: quranService && quranService.downloading

                Rectangle {
                    width: parent.width * (quranService && quranService.downloadTotal > 0 ? Math.min(1, quranService.downloadDone / quranService.downloadTotal) : 0)
                    height: parent.height
                    radius: Style.cornerRadius
                    color: root.accentC
                }

                Text {
                    anchors.centerIn: parent
                    text: quranService ? (quranService.downloadDone + " / " + quranService.downloadTotal) : ""
                    color: root.fg
                    font.family: root.barFontFamily
                    font.pixelSize: Style.font.bodySmall
                }
            }

            Row {
                visible: !(quranService && quranService.downloading)
                spacing: Style.space(6)

                Button {
                    text: root.tr("download")
                    foreground: root.fg
                    horizontalPadding: Style.spacing.controlPaddingX
                    verticalPadding: Style.spacing.controlPaddingY
                    onClicked: {
                        if (quranService && root.pendingDownloadReciter) {
                            quranService.downloadMushaf(root.pendingDownloadReciter.identifier);
                            // The prompt is only a first-selection decision;
                            // progress must not keep a modal panel over the UI.
                            root.pendingDownloadReciter = null;
                        }
                    }
                }

                Button {
                    text: root.tr("streamOnly")
                    foreground: root.fg
                    horizontalPadding: Style.spacing.controlPaddingX
                    verticalPadding: Style.spacing.controlPaddingY
                    onClicked: root.declineDownload()
                }

                Button {
                    text: root.tr("close")
                    foreground: root.fg
                    horizontalPadding: Style.spacing.controlPaddingX
                    verticalPadding: Style.spacing.controlPaddingY
                    onClicked: root.pendingDownloadReciter = null
                }
            }
        }
    }
}
