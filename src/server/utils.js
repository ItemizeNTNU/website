import {fetchResource} from "../utils/api";
import fetch from 'node-fetch';

export const permission = (...roles) => {
	return (req, res, next) => {
		if (!req.user) {
			return res.status(401).send({ message: 'You are not logged in' });
		}
		if (!roles.every((role) => req.user.roles.includes(role))) {
			return res.status(401).send({ message: 'Permission denied' });
		}
		next();
	};
};

export const removeEmptyObjects = (obj, cache) => {
	if (!Array.isArray(cache)) cache = [];
	if (cache.includes(obj)) return;
	cache.push(obj);
	for (let k of Object.keys(obj)) {
		if (typeof obj[k] == 'object') {
			if (Object.keys(obj[k]).length == 0) {
				delete obj[k];
			} else {
				removeEmptyObjects(obj[k], cache);
			}
		}
	}
};
//deletes discord event with id = eventId
export const deleteDiscordEvent = async (eventId) => {
	return await fetchResource(`https://discord.com/api/v8/guilds/${process.env.DISCORD_SERVER_ID}/scheduled-events/${eventId}`, 
        { method: 'DELETE', errorText: 'ERROR', fetch: fetch,
            headers: {
                "Authorization": `Bot ${process.env.DISCORD_BOT_TOKEN}`
            }
        });
};
//makes a discord event when an event is made on the website, takes in form data from the make-event form.
//if isUpdate == true it updates an existing event instead
export const makeDiscordEvent = async (jsonData, isUpdate) => {
    //convert to iso8601 timestamp as thats what the discord api requires
    const eventStartIso8601 = new Date(jsonData.date).toISOString()
    const eventEndIso8601 = new Date(jsonData.end).toISOString()
    const json = {
        "name" : `${jsonData.name}`,
        "entity_metadata": {
            "location": `${jsonData.location.name}`
        },
        "scheduled_start_time": `${eventStartIso8601}`,
        "scheduled_end_time": `${eventEndIso8601}`,
        //entity_type: 3 means externel event, eg not in the discord channel
        "entity_type": 3,
        //level 2 = channel members only, only available at time of creation
        "privacy_level": 2,
    }

    let descriptionText = jsonData.info;
    //these fields are not required
    if(jsonData.register_url){
        descriptionText += `\nregister at du kommer her: ${jsonData.location.url}`
    }
    if(jsonData.location.url){
        descriptionText += `\nlink til lokasjon: ${jsonData.location.url}`
    }
    if(jsonData.ctf.name){
        descriptionText += `\nctf navn: ${jsonData.ctf.name}`
    }
    if(jsonData.ctf.url){
        descriptionText += `\nctf link: ${jsonData.ctf.url}`
    }
    json["description"] = descriptionText

    let path = `https://discord.com/api/v8/guilds/${process.env.DISCORD_SERVER_ID}/scheduled-events`;
    let method = "POST";
    //its an update
    if(isUpdate){
        path = `https://discord.com/api/v8/guilds/${process.env.DISCORD_SERVER_ID}/scheduled-events/${jsonData.discordEventId}`;
        method = "PATCH";
    };

    const fetchOptions = {
        host: '',
        method: method,
        json:json,  
        /** Only works for one layer! */
        urlData: '',
        errorText: 'Unable to fetch resource: ERROR',
        fetch: fetch,
        headers: {
            'Authorization': `Bot ${process.env.DISCORD_BOT_TOKEN}`
        }
    };
    let svar = await fetchResource(path, fetchOptions);

    if (svar.exception === true || !svar.ok){
        return {exception: true, error: "Eventet kunne ikke lages på discord"}
    }
    //returns discord id
    return {exception: false, event_id: svar.json.id}
}
export const cookieRecipe = (req, res, next) => {
	const recipe =
		'MjI1ZyByb210ZW1wZXJlcnQgbWVpZXJpc234cgoyMDBnIHN1a2tlcgoyMDBnIHLlcvhyc3Vra2VyCjIgc3RrIGVnZwo0MDBnIGh2ZXRlbWVsCjAuNSB0cyBiYWtlcHVsdmVyCjEgdHMgbmF0cm9uCjEgdHMgc2FsdAoyIHRzIHZhbmlsamVzdWtrZXIKMzUwZyBzam9rb2xhZGUgKG34cmsgZWxsZXIgaGFsdm34cmspCgoKMS4gRm9ydmFybSBvdm5lbiB0aWwgMTkwQy4gUGlzayByb210ZW1wZXJlcnQgc234ciwgc3Vra2VyIG9nIHLlcvhyc3Vra2VyIGh2aXR0IGkgZW4ga2r4a2tlbm1hc2tpbiBlbGxlciBtZWQgZW4gZWxla3RyaXNrIHZpc3AuCgoyLiBIYSBpIGV0dCBlZ2cgb20gZ2FuZ2VuLCBvZyBwaXNrIGdvZHQgbWVsbG9tIGh2ZXJ0IGVnZy4gVGlsc2V0dCBtZWwsIGJha2VwdWx2ZXIsIG5hdHJvbiwgc2FsdCBvZyB2YW5pbGplc3Vra2VyIG9nIGtq-HIgdGlsIGdvZHQgYmxhbmRldC4KCjMuIEdyb3ZoYWtrIHNqb2tvbGFkZSBvZyB2ZW5kIGlubi4gU3BhciBldmVudHVlbHQgbm9lIHRpbCBweW50IHDlIHRvcHBlbi4KCjQuIFNldHQgMS0yIHNwaXNlc2tqZWVyIGRlaWcgcOUgc3Rla2VicmV0dCBkZWtrZXQgbWVkIGJha2VwYXBpci4gU_hyZyBmb3IgYXQgZGV0IGVyIGdvZHQgbWVkIHJvbSBtZWxsb20gaHZlciBramVrcyBkYSBkZSBmbHl0ZXIgdXRvdmVyLCBjYS4gNiBwZXIgYnJldHQuCgo1LiBTdGlrayBub2VuIHNqb2tvbGFkZWJpdGVyIHDlIHRvcHBlbiBhdiBodmVyIGtqZWtzIG9nIHN0ZWsgaSBvdm5lbiBpIGNhLiAxMi0xNSBtaW51dHRlciBlbGxlciB0aWwgZ3lsbmUuIEF2a2r4bCBsaXR0IHDlIHBsYXRlbiBm-HIgZGUgZmx5dHRlcyBvdmVyIHDlIHJpc3Qu';
	if (!req.headers.cookie?.includes(recipe)) {
		res.cookie('cookies', recipe);
	}
	next();
};
