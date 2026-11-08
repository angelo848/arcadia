export namespace main {
	
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

