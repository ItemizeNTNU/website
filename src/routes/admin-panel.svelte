<script context="module">
	import api from '../utils/api';

	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, '/login');
		} else if (!session.user?.roles.includes('Styret')) {
			this.redirect(401, '/');
		}
		const query = 'queryString=*';
		const dbUsers = await api.searchUsers(query, { fetch: this.fetch });
		const dbApplications = await api.getAllApplications({ fetch: this.fetch });
		const dbGroups = await api.getAllGroups({ fetch: this.fetch });
		return { users: dbUsers.json.users, applications: dbApplications.json.applications, groups: dbGroups.json.groups };
	}
</script>

<script>
	import { getModal } from '../components/PopupModal.svelte';
	import EditUserPopup from '../components/EditUserPopup.svelte';
	import AdminPanelTable from '../components/AdminPanelTable.svelte';
	import date from '../utils/date';
	import FaEdit from 'svelte-icons/fa/FaEdit.svelte';
	import FaRegPlusSquare from 'svelte-icons/fa/FaRegPlusSquare.svelte';
	import FaRegMinusSquare from 'svelte-icons/fa/FaRegMinusSquare.svelte';
	export let users;
	export let applications;
	export let groups;

	let attributeToEdit = '';
	let editType = '';
	// The selected columns to show in table
	let selectedCols = ['fullName', 'discordName', 'email', 'type'];
	// The selected filters to show in advanced filter
	let selectedFilters = ['fullName', 'displayName', 'discordName', 'email', 'registerd', 'type', 'groups', 'applications'];

	/* Object in DATA
	{
		key: identifier of attribute for user in users
		title: name of the attribute
		value: function to retrive the value from a user object in users
		defaultFilterting: default value to filter the attribute on 
		filter: function that returns if the user matches the filter for that attribute
		filterValues: list of the possible values this attribute can be filtered on
		edit: Possible values that this attribute can be set to, '' indicates all strings
		editConfirm: function to retrive a valid user object to PATCH user after confirmation 
		isDate: is the attribute a date
		add: function that returns all valid values that can be added to this user
	}
	*/
	const DATA = {
		fullName: {
			key: 'fullName',
			title: 'Navn',
			value: (r) => r.fullName,
			defaultFiltering: '',
			filter: (r, v) => r.fullName.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0
		},
		displayName: {
			key: 'displayName',
			title: 'Visningsnavn',
			value: (r) => r.displayName || '',
			defaultFiltering: '',
			filter: (r, v) => r.displayName.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0,
			edit: '',
			editConfirm: (v) => {
				return { user: { data: { displayName: v } } };
			}
		},
		discordName: {
			key: 'discordName',
			title: 'Discord-navn',
			value: (r) => r.discordUsername || '',
			defaultFiltering: '',
			filter: (r, v) => (r.discordUsername || '').toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0
		},
		email: {
			key: 'email',
			title: 'E-post',
			value: (r) => r.email,
			defaultFiltering: '',
			filter: (r, v) => r.email.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0
		},
		registerd: {
			key: 'insertInstant',
			title: 'Registrert',
			value: (r) => r.insertInstant,
			defaultFiltering: ['', ''],
			filter: (r, v) => r.insertInstant <= (new Date(v[0]).getTime() || r.insertInstant) && r.insertInstant >= (new Date(v[1]).getTime() || 0),
			isDate: true
		},
		type: {
			key: 'type',
			title: 'Medlemstype',
			value: (r) => r.type || '',
			defaultFiltering: [],
			filterValues: ['alumni', 'employee', 'student'],
			filter: (r, v) => v?.includes(r.type) || v?.length == 0
		},

		study_program: {
			key: 'program',
			title: 'Studieretning',
			value: (r) => r.study?.program || '',
			edit: '',
			editConfirm: (v) => {
				return { user: { data: { study: { program: v } } } };
			}
		},
		study_year: {
			key: 'year',
			title: 'Årstrinn',
			value: (r) => r.study?.year || '',
			edit: [1, 2, 3, 4, 5, 6, 7, 8],
			editConfirm: (v) => {
				return { user: { data: { study: { year: v } } } };
			}
		},
		groups: {
			key: 'groupIds',
			title: 'Gruppe',
			title_plural: 'Grupper',
			value: (r) => r.groupIds?.map((id) => groups[id]?.name),
			add: (r) =>
				Object.keys(groups)
					.filter((id) => !r.groupIds?.includes(id))
					?.map((id) => {
						let name = groups[id].name;
						return { id, name };
					}),
			addConfirm: (userId, groupId) => {
				let members = {};
				members[groupId] = [{ userId: userId }];
				return { members };
			},
			delete: (r) =>
				r?.groupIds?.map((g) => {
					return { id: g, name: groups[g].name };
				}) || [],
			deleteConfirm: (userId, groupId) => {
				let members = {};
				members[groupId] = [userId];
				return { members };
			},
			defaultFiltering: [],
			filterValues: Object.keys(groups).map((id) => groups[id].name),
			filter: (r, v) => r.groupIds?.find((id) => v?.includes(groups[id].name)) || v?.length == 0
		},
		applications: {
			key: 'applicationRoles',
			title: 'Applikasjon',
			title_plural: 'Applikasjoner',
			value: (r) => r.applicationRoles?.map((a) => applications[a.id]?.name),
			add: (r) =>
				Object.keys(applications)
					.filter((id) => !r.applicationRoles?.find((a) => a.id == id))
					?.map((id) => applications[id].name),
			defaultFiltering: [],
			filterValues: Object.keys(applications).map((id) => applications[id].name),
			filter: (r, v) => r.applicationRoles?.find((app) => v?.includes(applications[app.id].name)) || v?.length == 0
		}
	};
	const simpleFilter = {
		title: 'Søk etter brukere (navn, discord-navn og epost): ',
		filter: (r, v) => [r.name, r.discordUsername, r.email].find((e) => e?.toLocaleLowerCase().indexOf(v.toLocaleLowerCase()) >= 0)
	};

	$: filters = selectedFilters.map((key) => DATA[key]);
	$: cols = selectedCols.map((key) => DATA[key]);

	const editUser = (type, active) => {
		attributeToEdit = active;
		editType = type;
		getModal('first').open();
	};
	async function editValue(event) {
		let row = event.detail.row;
		const updatedUser = await api.patchUser(row.id, event.detail.edit, { fetch: this.fetch });
		// update rows with updated user
		if (updatedUser) users = users.map((u) => (u.id == row.id ? updatedUser.json.user : u));
		// TODO: Add confirmation of edit or error
	}
	async function addValue(event) {
		let row = event.detail.row;
		const resp = await api.addUsersToGroup(event.detail.add, { fetch: this.fetch });
		if (resp.ok) {
			let groupIds = row.groupIds || [];
			groupIds.push(Object.keys(event.detail.add.members)[0]);
			users.forEach((u) => {
				if (u.id == row.id) u.groupIds = groupIds;
			});
			// To force update the table
			users = users;
		}
	}
	async function deleteValue(event) {
		let row = event.detail.row;
		const resp = await api.removeUsersFromGroup(event.detail.delete, { fetch: this.fetch });
		if (resp.ok) {
			let groupIds = row.groupIds.filter((g) => g != Object.keys(event.detail.delete.members)[0]);
			if (groupIds.length == 0) groupIds = undefined;
			users.forEach((u) => {
				if (u.id == row.id) u.groupIds = groupIds;
			});
			// To force update the table
			users = users;
		}
	}
</script>

<svelte:head>
	<title>Admin panel</title>
</svelte:head>

<main>
	<div class="container">
		<div class="row">
			<div class="main-info">
				<h2>Totalt antall medlemmer: {users.length}</h2>
				<span>Studenter: {users.filter((e) => e.type == 'student').length}</span>
				<span>Alumni: {users.filter((e) => e.type == 'alumni').length}</span>
				<span>Ansatte: {users.filter((e) => e.type == 'employee').length}</span>
			</div>
			<br />
			<AdminPanelTable columns={cols} rows={users} {filters} expandRowKey="fullName" simpleFilterOptions={simpleFilter}>
				<div slot="expanded" let:row class="user-info">
					<p><b>Fullt Navn: </b>{row.fullName}</p>
					<p><b>Visningsnavn: </b>{row.displayName} <span on:click={() => editUser('edit', DATA['displayName'])}><FaEdit /></span></p>
					<p><b>E-post: </b>{row.email}</p>
					<p><b>Medlemstype: </b>{row.type || ''}</p>
					{#if row.type == 'student' || row.type == 'alumni'}
						<p><b>Studieretning: </b>{row.study.program} <span on:click={() => editUser('edit', DATA['study_program'])}><FaEdit /></span></p>
						{#if row.type == 'student'}
							<p><b>Årstrinn: </b>{row.study.year} <span on:click={() => editUser('edit', DATA['study_year'])}><FaEdit /></span></p>
						{:else}
							<p><b>Medlemsår: </b>{row.alumni.joinYear}</p>
						{/if}
					{:else if row.type == 'employee'}
						<p><b>Title: </b>{row.employee.title}</p>
						<p />
					{/if}
					<div class="user-roles">
						<p>
							<b>Grupper</b>
							<span class="green" on:click={() => editUser('add', DATA['groups'])}><FaRegPlusSquare /></span>
							<span class="red" on:click={() => editUser('delete', DATA['groups'])}><FaRegMinusSquare /></span>
						</p>
						<hr />
						{#if row?.groupIds}
							<div class="role-info">
								<p><b>Navn:</b></p>
								<p><b>Tilgang til applikasjoner:</b></p>
								{#each row.groupIds as id}
									<p>{groups[id]?.name}</p>
									<p>
										{Object.keys(groups[id]?.roles)
											.map((e) => applications[e].name)
											.join(', ')}
									</p>
								{/each}
							</div>
						{:else}
							<p><i>Ikke medlem av noen grupper</i></p>
						{/if}
					</div>
					<div class="user-roles">
						<p><b>Applikasjoner</b><span class="green" on:click={() => editUser('add', DATA['applications'])}><FaRegPlusSquare /></span></p>
						<hr />
						{#if row?.applicationRoles}
							<div class="role-info">
								<span><b>Navn:</b></span><span><b>Registrert med roller:</b></span>
								{#each row.applicationRoles as application}
									<p>{applications[application.id]?.name}</p>
									<p>{application.roles.join(', ')}</p>
								{/each}
							</div>
						{:else}
							<p><i>Ikke tilgang til noen applikasjoner</i></p>
						{/if}
					</div>
					<p><b>Bruker opprettet: </b>{date.nicePrintDate(new Date(row.insertInstant))}</p>
					<p><b>Sist logget in: </b>{date.nicePrintDate(new Date(row.lastLoginInstant))}</p>
					<EditUserPopup {attributeToEdit} {row} {editType} on:edit={editValue} on:add={addValue} on:delete={deleteValue} />
				</div>
			</AdminPanelTable>
		</div>
	</div>
</main>

<style>
	.main-info {
		display: grid;
		grid-template-columns: 40% 20% 20% 20%;
		align-items: top;
		padding: 0.6em;
	}
	.user-info {
		display: grid;
		grid-template-columns: 50% 50%;
		align-items: top;
		padding: 0.6em;
	}
	.user-info * {
		margin: 0.1em 0;
	}
	.user-info p,
	div {
		width: 100%;
	}
	hr {
		display: block;
		height: 1px;
		border: 0;
		width: 70%;
		border-top: 1px solid #ccc;
		margin: 1em 0;
		padding: 0;
	}
	.user-roles {
		padding: 5px;
	}
	.user-roles p {
		padding: 5px;
	}
	.role-info {
		display: grid;
		grid-template-columns: 25% 75%;
		align-items: top;
		margin: 0;
		padding: 0;
	}
	.role-info p {
		width: 100%;
		padding: 0px;
		padding-left: 5px;
	}
	.user-info > p > span,
	.user-roles > p > span {
		display: inline-block;
		height: 0.75em;
		vertical-align: baseline;
		margin-left: 4px;
		color: #fff;
		transition: 0.4s linear;
	}
	span.red:hover {
		color: var(--error);
	}
	.user-info > p > span:hover,
	.user-roles > p > span.green:hover {
		color: var(--green-2);
	}
</style>
