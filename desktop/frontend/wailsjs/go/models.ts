export namespace ipc {
	
	export class Status {
	    phase: string;
	    failure?: string;
	    message?: string;
	    step?: string;
	    profileId?: string;
	    profileName?: string;
	    server?: string;
	    vpnAddress?: string;
	    connectedAt?: number;
	    bytesIn: number;
	    bytesOut: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.failure = source["failure"];
	        this.message = source["message"];
	        this.step = source["step"];
	        this.profileId = source["profileId"];
	        this.profileName = source["profileName"];
	        this.server = source["server"];
	        this.vpnAddress = source["vpnAddress"];
	        this.connectedAt = source["connectedAt"];
	        this.bytesIn = source["bytesIn"];
	        this.bytesOut = source["bytesOut"];
	    }
	}

}

export namespace main {
	
	export class ManagedView {
	    name: string;
	    profileId: string;
	    remote: string;
	    links: setupfile.Link[];
	    hasTotp: boolean;
	    hasPassword: boolean;
	    manualCode: boolean;
	    codeLength: number;
	    needsUser: boolean;
	    needsCredentials: boolean;
	    lockMethod: string;
	
	    static createFrom(source: any = {}) {
	        return new ManagedView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.profileId = source["profileId"];
	        this.remote = source["remote"];
	        this.links = this.convertValues(source["links"], setupfile.Link);
	        this.hasTotp = source["hasTotp"];
	        this.hasPassword = source["hasPassword"];
	        this.manualCode = source["manualCode"];
	        this.codeLength = source["codeLength"];
	        this.needsUser = source["needsUser"];
	        this.needsCredentials = source["needsCredentials"];
	        this.lockMethod = source["lockMethod"];
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
	export class ProfileView {
	    id: string;
	    name: string;
	    remote: string;
	    username: string;
	    needsCredentials: boolean;
	    twoFactor: boolean;
	    codeAfter: boolean;
	    codeLength: number;
	    staticChallenge: boolean;
	    rememberPassword: boolean;
	    importedAt: number;
	    managed: boolean;
	    hasSavedPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.remote = source["remote"];
	        this.username = source["username"];
	        this.needsCredentials = source["needsCredentials"];
	        this.twoFactor = source["twoFactor"];
	        this.codeAfter = source["codeAfter"];
	        this.codeLength = source["codeLength"];
	        this.staticChallenge = source["staticChallenge"];
	        this.rememberPassword = source["rememberPassword"];
	        this.importedAt = source["importedAt"];
	        this.managed = source["managed"];
	        this.hasSavedPassword = source["hasSavedPassword"];
	    }
	}
	export class AppState {
	    version: string;
	    platform: string;
	    language: string;
	    initError?: string;
	    helperUp: boolean;
	    helperVersion: string;
	    status: ipc.Status;
	    profiles: ProfileView[];
	    managed?: ManagedView;
	    locked: boolean;
	    helloAvailable: boolean;
	    weakKeyStorage: boolean;
	    retryIn: number;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.platform = source["platform"];
	        this.language = source["language"];
	        this.initError = source["initError"];
	        this.helperUp = source["helperUp"];
	        this.helperVersion = source["helperVersion"];
	        this.status = this.convertValues(source["status"], ipc.Status);
	        this.profiles = this.convertValues(source["profiles"], ProfileView);
	        this.managed = this.convertValues(source["managed"], ManagedView);
	        this.locked = source["locked"];
	        this.helloAvailable = source["helloAvailable"];
	        this.weakKeyStorage = source["weakKeyStorage"];
	        this.retryIn = source["retryIn"];
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
	export class ConnectInput {
	    username: string;
	    password: string;
	    code: string;
	    remember: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConnectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.password = source["password"];
	        this.code = source["code"];
	        this.remember = source["remember"];
	    }
	}
	export class Draft {
	    id: string;
	    name: string;
	    remote: string;
	    needsCredentials: boolean;
	    staticChallenge: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Draft(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.remote = source["remote"];
	        this.needsCredentials = source["needsCredentials"];
	        this.staticChallenge = source["staticChallenge"];
	    }
	}
	export class ImportResult {
	    kind: string;
	    path?: string;
	    draft?: Draft;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.draft = this.convertValues(source["draft"], Draft);
	        this.error = source["error"];
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
	
	export class ProfileInput {
	    name: string;
	    username: string;
	    twoFactor: boolean;
	    codeAfter: boolean;
	    codeLength: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.username = source["username"];
	        this.twoFactor = source["twoFactor"];
	        this.codeAfter = source["codeAfter"];
	        this.codeLength = source["codeLength"];
	    }
	}
	
	export class SetupSummary {
	    name: string;
	    remote: string;
	    hasTotp: boolean;
	    hasPassword: boolean;
	    manualCode: boolean;
	    links: setupfile.Link[];
	    needsLock: boolean;
	    replaces?: string;
	
	    static createFrom(source: any = {}) {
	        return new SetupSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.remote = source["remote"];
	        this.hasTotp = source["hasTotp"];
	        this.hasPassword = source["hasPassword"];
	        this.manualCode = source["manualCode"];
	        this.links = this.convertValues(source["links"], setupfile.Link);
	        this.needsLock = source["needsLock"];
	        this.replaces = source["replaces"];
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
	export class UnlockResult {
	    ok: boolean;
	    attemptsLeft: number;
	    lockedUntil: number;
	    wiped: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UnlockResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.attemptsLeft = source["attemptsLeft"];
	        this.lockedUntil = source["lockedUntil"];
	        this.wiped = source["wiped"];
	    }
	}

}

export namespace setupfile {
	
	export class Link {
	    t: string;
	    k: string;
	    u: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.t = source["t"];
	        this.k = source["k"];
	        this.u = source["u"];
	    }
	}

}

