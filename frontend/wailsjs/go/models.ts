export namespace main {
	
	export class ProviderView {
	    enabled: boolean;
	    hasKey: boolean;
	    keyMask: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.hasKey = source["hasKey"];
	        this.keyMask = source["keyMask"];
	    }
	}
	export class ProvidersView {
	    glm: ProviderView;
	    ark: ProviderView;
	    deepseek: ProviderView;
	    kimi: ProviderView;
	    stepfun: ProviderView;
	    minimax: ProviderView;
	
	    static createFrom(source: any = {}) {
	        return new ProvidersView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.glm = this.convertValues(source["glm"], ProviderView);
	        this.ark = this.convertValues(source["ark"], ProviderView);
	        this.deepseek = this.convertValues(source["deepseek"], ProviderView);
	        this.kimi = this.convertValues(source["kimi"], ProviderView);
	        this.stepfun = this.convertValues(source["stepfun"], ProviderView);
	        this.minimax = this.convertValues(source["minimax"], ProviderView);
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
	export class ConfigView {
	    providers: ProvidersView;
	    refreshIntervalMinutes: number;
	    notifyOnReset: boolean;
	    alertPercentThreshold: number;
	    alertBalanceThreshold: number;
	
	    static createFrom(source: any = {}) {
	        return new ConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providers = this.convertValues(source["providers"], ProvidersView);
	        this.refreshIntervalMinutes = source["refreshIntervalMinutes"];
	        this.notifyOnReset = source["notifyOnReset"];
	        this.alertPercentThreshold = source["alertPercentThreshold"];
	        this.alertBalanceThreshold = source["alertBalanceThreshold"];
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
	
	
	export class UsageItemView {
	    label: string;
	    used: number;
	    limit: number;
	    format: string;
	    unit: string;
	    percent: number;
	    resetsIn: string;
	    noAlert: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UsageItemView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.used = source["used"];
	        this.limit = source["limit"];
	        this.format = source["format"];
	        this.unit = source["unit"];
	        this.percent = source["percent"];
	        this.resetsIn = source["resetsIn"];
	        this.noAlert = source["noAlert"];
	    }
	}
	export class RefreshResult {
	    providerName: string;
	    plan: string;
	    items: UsageItemView[];
	    error?: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RefreshResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerName = source["providerName"];
	        this.plan = source["plan"];
	        this.items = this.convertValues(source["items"], UsageItemView);
	        this.error = source["error"];
	        this.updatedAt = source["updatedAt"];
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
	export class RefreshResponse {
	    results: RefreshResult[];
	    notifications: string[];
	    updatedAt: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RefreshResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.results = this.convertValues(source["results"], RefreshResult);
	        this.notifications = source["notifications"];
	        this.updatedAt = source["updatedAt"];
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
	

}

