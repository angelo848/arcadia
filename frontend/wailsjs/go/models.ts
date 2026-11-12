export namespace main {
	
	export class PlatformView {
	    id: number;
	    name: string;
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new PlatformView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	    }
	}
	export class CatalogView {
	    platforms: PlatformView[];
	    genres: string[];
	    companies: string[];
	    playModes: string[];
	
	    static createFrom(source: any = {}) {
	        return new CatalogView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platforms = this.convertValues(source["platforms"], PlatformView);
	        this.genres = source["genres"];
	        this.companies = source["companies"];
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
	    description: string;
	    completedAt: string;
	    developers: string[];
	    publishers: string[];
	    genres: string[];
	    playModes: string[];
	
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
	        this.description = source["description"];
	        this.completedAt = source["completedAt"];
	        this.developers = source["developers"];
	        this.publishers = source["publishers"];
	        this.genres = source["genres"];
	        this.playModes = source["playModes"];
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
	export class PickedExe {
	    path: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new PickedExe(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.title = source["title"];
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

