export namespace main {
	
	export class CatalogItemView {
	    id: number;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new CatalogItemView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class PlatformView {
	    id: number;
	    name: string;
	    category: string;
	    emulatorId?: number;
	
	    static createFrom(source: any = {}) {
	        return new PlatformView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.emulatorId = source["emulatorId"];
	    }
	}
	export class CatalogView {
	    platforms: PlatformView[];
	    genres: CatalogItemView[];
	    companies: CatalogItemView[];
	    playModes: string[];
	
	    static createFrom(source: any = {}) {
	        return new CatalogView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platforms = this.convertValues(source["platforms"], PlatformView);
	        this.genres = this.convertValues(source["genres"], CatalogItemView);
	        this.companies = this.convertValues(source["companies"], CatalogItemView);
	        this.playModes = source["playModes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EmulatorView {
	    id: number;
	    name: string;
	    exePath: string;
	    argsTemplate: string;
	
	    static createFrom(source: any = {}) {
	        return new EmulatorView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.exePath = source["exePath"];
	        this.argsTemplate = source["argsTemplate"];
	    }
	}
	export class EmulatorListView {
	    emulators: EmulatorView[];
	
	    static createFrom(source: any = {}) {
	        return new EmulatorListView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.emulators = this.convertValues(source["emulators"], EmulatorView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class GameDetailView {
	    id: number;
	    platformId: number;
	    title: string;
	    platform: string;
	    path: string;
	    status: string;
	    favorite: boolean;
	    year?: number;
	    totalPlaySeconds: number;
	    playCount: number;
	    firstPlayedAt: string;
	    lastPlayedAt: string;
	    emulatorId?: number;
	    description: string;
	    completedAt: string;
	    developers: string[];
	    publishers: string[];
	    genres: string[];
	    playModes: string[];
	    launchType: string;
	    launchArgs: string;
	
	    static createFrom(source: any = {}) {
	        return new GameDetailView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.platformId = source["platformId"];
	        this.title = source["title"];
	        this.platform = source["platform"];
	        this.path = source["path"];
	        this.status = source["status"];
	        this.favorite = source["favorite"];
	        this.year = source["year"];
	        this.totalPlaySeconds = source["totalPlaySeconds"];
	        this.playCount = source["playCount"];
	        this.firstPlayedAt = source["firstPlayedAt"];
	        this.lastPlayedAt = source["lastPlayedAt"];
	        this.emulatorId = source["emulatorId"];
	        this.description = source["description"];
	        this.completedAt = source["completedAt"];
	        this.developers = source["developers"];
	        this.publishers = source["publishers"];
	        this.genres = source["genres"];
	        this.playModes = source["playModes"];
	        this.launchType = source["launchType"];
	        this.launchArgs = source["launchArgs"];
	    }
	}
	export class GameView {
	    id: number;
	    platformId: number;
	    title: string;
	    platform: string;
	    path: string;
	    status: string;
	    favorite: boolean;
	    year?: number;
	    totalPlaySeconds: number;
	    playCount: number;
	    firstPlayedAt: string;
	    lastPlayedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new GameView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.platformId = source["platformId"];
	        this.title = source["title"];
	        this.platform = source["platform"];
	        this.path = source["path"];
	        this.status = source["status"];
	        this.favorite = source["favorite"];
	        this.year = source["year"];
	        this.totalPlaySeconds = source["totalPlaySeconds"];
	        this.playCount = source["playCount"];
	        this.firstPlayedAt = source["firstPlayedAt"];
	        this.lastPlayedAt = source["lastPlayedAt"];
	    }
	}
	export class GameListView {
	    games: GameView[];
	    total: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new GameListView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.games = this.convertValues(source["games"], GameView);
	        this.total = source["total"];
	        this.offset = source["offset"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ImportAppliedView {
	    gameId: number;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportAppliedView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameId = source["gameId"];
	        this.title = source["title"];
	    }
	}
	export class ImportSkippedView {
	    title: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportSkippedView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.reason = source["reason"];
	    }
	}
	export class ImportApplyView {
	    applied: ImportAppliedView[];
	    skipped: ImportSkippedView[];
	
	    static createFrom(source: any = {}) {
	        return new ImportApplyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.applied = this.convertValues(source["applied"], ImportAppliedView);
	        this.skipped = this.convertValues(source["skipped"], ImportSkippedView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportCandidateView {
	    source: string;
	    title: string;
	    platform: string;
	    launchType: string;
	    launchTarget: string;
	    installDir: string;
	    developers: string[];
	    publishers: string[];
	    genres: string[];
	    year?: number;
	    description: string;
	    playModes: string[];
	    playedSeconds?: number;
	    lastPlayed?: string;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportCandidateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.title = source["title"];
	        this.platform = source["platform"];
	        this.launchType = source["launchType"];
	        this.launchTarget = source["launchTarget"];
	        this.installDir = source["installDir"];
	        this.developers = source["developers"];
	        this.publishers = source["publishers"];
	        this.genres = source["genres"];
	        this.year = source["year"];
	        this.description = source["description"];
	        this.playModes = source["playModes"];
	        this.playedSeconds = source["playedSeconds"];
	        this.lastPlayed = source["lastPlayed"];
	        this.exists = source["exists"];
	    }
	}
	export class ImportScanView {
	    candidates: ImportCandidateView[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportScanView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.candidates = this.convertValues(source["candidates"], ImportCandidateView);
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LibraryQuery {
	    filter: number[];
	    sort: string;
	    offset: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new LibraryQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filter = source["filter"];
	        this.sort = source["sort"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	    }
	}
	export class MediaItemView {
	    id: number;
	    kind: string;
	    provider: string;
	    path: string;
	    thumbPath: string;
	    url: string;
	    thumbUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new MediaItemView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.provider = source["provider"];
	        this.path = source["path"];
	        this.thumbPath = source["thumbPath"];
	        this.url = source["url"];
	        this.thumbUrl = source["thumbUrl"];
	    }
	}
	export class MediaListView {
	    media: MediaItemView[];
	
	    static createFrom(source: any = {}) {
	        return new MediaListView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.media = this.convertValues(source["media"], MediaItemView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ProviderKeyView {
	    provider: string;
	    configured: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProviderKeyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.configured = source["configured"];
	    }
	}
	export class ScrapeApplyView {
	    game: GameDetailView;
	    queued: number;
	
	    static createFrom(source: any = {}) {
	        return new ScrapeApplyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game = this.convertValues(source["game"], GameDetailView);
	        this.queued = source["queued"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ScrapeCandidateView {
	    providerId: string;
	    title: string;
	    platform: string;
	    year: number;
	    score: number;
	    accepted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScrapeCandidateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.title = source["title"];
	        this.platform = source["platform"];
	        this.year = source["year"];
	        this.score = source["score"];
	        this.accepted = source["accepted"];
	    }
	}
	export class ScrapeSearchView {
	    candidates: ScrapeCandidateView[];
	
	    static createFrom(source: any = {}) {
	        return new ScrapeSearchView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.candidates = this.convertValues(source["candidates"], ScrapeCandidateView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionItemView {
	    sessionId: number;
	    gameId: number;
	    state: string;
	    startedAt: string;
	    endedAt: string;
	    activeSeconds: number;
	    endReason: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionItemView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.gameId = source["gameId"];
	        this.state = source["state"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.activeSeconds = source["activeSeconds"];
	        this.endReason = source["endReason"];
	    }
	}
	export class SessionListView {
	    sessions: SessionItemView[];
	    total: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionListView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], SessionItemView);
	        this.total = source["total"];
	        this.offset = source["offset"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionView {
	    sessionId: number;
	    gameId: number;
	    title: string;
	    state: string;
	    startedAt: string;
	    endedAt: string;
	    activeSeconds: number;
	    activeMs: number;
	    asOf: string;
	    tracking: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.gameId = source["gameId"];
	        this.title = source["title"];
	        this.state = source["state"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.activeSeconds = source["activeSeconds"];
	        this.activeMs = source["activeMs"];
	        this.asOf = source["asOf"];
	        this.tracking = source["tracking"];
	    }
	}
	export class StatsGameView {
	    gameId: number;
	    title: string;
	    platform: string;
	    sessions: number;
	    activeSeconds: number;
	    importedSeconds: number;
	    lastPlayedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new StatsGameView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameId = source["gameId"];
	        this.title = source["title"];
	        this.platform = source["platform"];
	        this.sessions = source["sessions"];
	        this.activeSeconds = source["activeSeconds"];
	        this.importedSeconds = source["importedSeconds"];
	        this.lastPlayedAt = source["lastPlayedAt"];
	    }
	}
	export class StatsPlatformView {
	    platformId: number;
	    name: string;
	    activeSeconds: number;
	    importedSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new StatsPlatformView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platformId = source["platformId"];
	        this.name = source["name"];
	        this.activeSeconds = source["activeSeconds"];
	        this.importedSeconds = source["importedSeconds"];
	    }
	}
	export class StatsSummaryView {
	    games: StatsGameView[];
	    platforms: StatsPlatformView[];
	    totalSeconds: number;
	    importedSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new StatsSummaryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.games = this.convertValues(source["games"], StatsGameView);
	        this.platforms = this.convertValues(source["platforms"], StatsPlatformView);
	        this.totalSeconds = source["totalSeconds"];
	        this.importedSeconds = source["importedSeconds"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StatsWeekView {
	    start: string;
	    seconds: number;
	    current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StatsWeekView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.seconds = source["seconds"];
	        this.current = source["current"];
	    }
	}
	export class StatsWeeklyView {
	    weeks: StatsWeekView[];
	
	    static createFrom(source: any = {}) {
	        return new StatsWeeklyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weeks = this.convertValues(source["weeks"], StatsWeekView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StatusView {
	    connected: boolean;
	    version: string;
	    pid: number;
	    startedAt: string;
	    dataDir: string;
	    uptimeSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new StatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.version = source["version"];
	        this.pid = source["pid"];
	        this.startedAt = source["startedAt"];
	        this.dataDir = source["dataDir"];
	        this.uptimeSeconds = source["uptimeSeconds"];
	    }
	}

}

