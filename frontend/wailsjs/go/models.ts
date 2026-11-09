export namespace main {
	
	export class GameView {
	    id: number;
	    title: string;
	    path: string;
	    totalPlaySeconds: number;
	    playCount: number;
	    lastPlayedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new GameView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.path = source["path"];
	        this.totalPlaySeconds = source["totalPlaySeconds"];
	        this.playCount = source["playCount"];
	        this.lastPlayedAt = source["lastPlayedAt"];
	    }
	}
	export class LibrarySummaryView {
	    games: number;
	    platforms: number;
	
	    static createFrom(source: any = {}) {
	        return new LibrarySummaryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.games = source["games"];
	        this.platforms = source["platforms"];
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

