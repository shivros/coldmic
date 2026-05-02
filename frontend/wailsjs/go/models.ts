export namespace domain {
	
	export class Status {
	    state: string;
	    active: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.active = source["active"];
	        this.message = source["message"];
	    }
	}
	export class StopResult {
	    rawTranscript: string;
	    finalTranscript: string;
	    copied: boolean;
	    sessionId?: string;
	
	    static createFrom(source: any = {}) {
	        return new StopResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rawTranscript = source["rawTranscript"];
	        this.finalTranscript = source["finalTranscript"];
	        this.copied = source["copied"];
	        this.sessionId = source["sessionId"];
	    }
	}

}

