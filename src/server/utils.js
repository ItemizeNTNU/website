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
export const cookieRecipe = (req, res, next) => {
	const recipe =
		'MjI1ZyByb210ZW1wZXJlcnQgbWVpZXJpc234cgoyMDBnIHN1a2tlcgoyMDBnIHLlcvhyc3Vra2VyCjIgc3RrIGVnZwo0MDBnIGh2ZXRlbWVsCjAuNSB0cyBiYWtlcHVsdmVyCjEgdHMgbmF0cm9uCjEgdHMgc2FsdAoyIHRzIHZhbmlsamVzdWtrZXIKMzUwZyBzam9rb2xhZGUgKG34cmsgZWxsZXIgaGFsdm34cmspCgoKMS4gRm9ydmFybSBvdm5lbiB0aWwgMTkwQy4gUGlzayByb210ZW1wZXJlcnQgc234ciwgc3Vra2VyIG9nIHLlcvhyc3Vra2VyIGh2aXR0IGkgZW4ga2r4a2tlbm1hc2tpbiBlbGxlciBtZWQgZW4gZWxla3RyaXNrIHZpc3AuCgoyLiBIYSBpIGV0dCBlZ2cgb20gZ2FuZ2VuLCBvZyBwaXNrIGdvZHQgbWVsbG9tIGh2ZXJ0IGVnZy4gVGlsc2V0dCBtZWwsIGJha2VwdWx2ZXIsIG5hdHJvbiwgc2FsdCBvZyB2YW5pbGplc3Vra2VyIG9nIGtq-HIgdGlsIGdvZHQgYmxhbmRldC4KCjMuIEdyb3ZoYWtrIHNqb2tvbGFkZSBvZyB2ZW5kIGlubi4gU3BhciBldmVudHVlbHQgbm9lIHRpbCBweW50IHDlIHRvcHBlbi4KCjQuIFNldHQgMS0yIHNwaXNlc2tqZWVyIGRlaWcgcOUgc3Rla2VicmV0dCBkZWtrZXQgbWVkIGJha2VwYXBpci4gU_hyZyBmb3IgYXQgZGV0IGVyIGdvZHQgbWVkIHJvbSBtZWxsb20gaHZlciBramVrcyBkYSBkZSBmbHl0ZXIgdXRvdmVyLCBjYS4gNiBwZXIgYnJldHQuCgo1LiBTdGlrayBub2VuIHNqb2tvbGFkZWJpdGVyIHDlIHRvcHBlbiBhdiBodmVyIGtqZWtzIG9nIHN0ZWsgaSBvdm5lbiBpIGNhLiAxMi0xNSBtaW51dHRlciBlbGxlciB0aWwgZ3lsbmUuIEF2a2r4bCBsaXR0IHDlIHBsYXRlbiBm-HIgZGUgZmx5dHRlcyBvdmVyIHDlIHJpc3Qu';
	if (!req.headers.cookie?.includes(recipe)) {
		res.cookie('cookies', recipe);
	}
	next();
};
