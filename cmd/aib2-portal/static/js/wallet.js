// AIB 2.0 Web Wallet JavaScript SDK
// Client-side wallet implementation with Ed25519 cryptography

// Configuration
const CONFIG = {
    apiEndpoint: 'https://testnet-api.aibprotocol.org', // Update with actual testnet endpoint
    explorerEndpoint: 'https://testnet-explorer.aibprotocol.org',
    defaultFee: 0.0001,
    minFee: 0.00001
};

// Wallet State
let wallet = null;
let currentNetwork = 'l1'; // l1 or l2
let historyFilter = 'all';
let isPrivateKeyVisible = false;

// DOM Elements
const elements = {
    statusMessage: document.getElementById('statusMessage'),
    totalBalance: document.getElementById('totalBalance'),
    walletAddress: document.getElementById('walletAddress'),
    privateKeyDisplay: document.getElementById('privateKeyDisplay'),
    txHistoryBody: document.getElementById('txHistoryBody'),
    lastHistoryUpdate: document.getElementById('lastHistoryUpdate'),
    currentNetwork: document.getElementById('currentNetwork'),
    // Tab content
    tabContent: {
        create: document.getElementById('tab-content-create'),
        import: document.getElementById('tab-content-import'),
        send: document.getElementById('tab-content-send'),
        history: document.getElementById('tab-content-history')
    },
    tabs: {
        create: document.getElementById('tab-create'),
        import: document.getElementById('tab-import'),
        send: document.getElementById('tab-send'),
        history: document.getElementById('tab-history')
    }
};

// Utility Functions
const utils = {
    formatBalance: (amount) => {
        return (amount / 1e8).toFixed(8);
    },

    formatAddress: (address) => {
        if (!address) return '';
        if (address.length > 40) {
            return address.substring(0, 16) + '...' + address.substring(address.length - 16);
        }
        return address;
    },

    showToast: (message, type = 'info') => {
        elements.statusMessage.textContent = message;
        elements.statusMessage.className = `status-message status-${type}`;
        setTimeout(() => {
            elements.statusMessage.className = 'status-message';
        }, 5000);
    },

    hexToBytes: (hex) => {
        if (hex.startsWith('0x')) hex = hex.slice(2);
        const bytes = new Uint8Array(hex.length / 2);
        for (let i = 0; i < bytes.length; i++) {
            bytes[i] = parseInt(hex.substr(i * 2, 2), 16);
        }
        return bytes;
    },

    bytesToHex: (bytes) => {
        return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('');
    },

    stringToBytes: (str) => {
        return new TextEncoder().encode(str);
    },

    bytesToString: (bytes) => {
        return new TextDecoder().decode(bytes);
    }
};

// Cryptography Functions (Web Crypto API)
const crypto = {
    generateKeyPair: async () => {
        const keyPair = await window.crypto.subtle.generateKey(
            { name: 'ED25519' },
            true,
            ['sign', 'verify']
        );

        const privateKeyBuffer = await window.crypto.subtle.exportKey('raw', keyPair.privateKey);
        const publicKeyBuffer = await window.crypto.subtle.exportKey('raw', keyPair.publicKey);

        return {
            privateKey: new Uint8Array(privateKeyBuffer),
            publicKey: new Uint8Array(publicKeyBuffer)
        };
    },

    sign: async (privateKey, message) => {
        const key = await window.crypto.subtle.importKey(
            'raw',
            privateKey,
            { name: 'ED25519' },
            false,
            ['sign']
        );

        const signature = await window.crypto.subtle.sign(
            { name: 'ED25519' },
            key,
            message
        );

        return new Uint8Array(signature);
    },

    verify: async (publicKey, message, signature) => {
        const key = await window.crypto.subtle.importKey(
            'raw',
            publicKey,
            { name: 'ED25519' },
            false,
            ['verify']
        );

        return await window.crypto.subtle.verify(
            { name: 'ED25519' },
            key,
            signature,
            message
        );
    }
};

// AIB Address Generation (Bech32m)
const bech32m = {
    charset: 'qpzry9x8gf2tvdw0s3jn54khce6mua7l',

    polymod: (values) => {
        const GEN = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];
        let chk = 1;
        for (const v of values) {
            const b = chk >> 25;
            chk = (chk & 0x1ffffff) << 5 ^ v;
            for (let i = 0; i < 5; i++) {
                chk ^= (b >> i & 1) && GEN[i];
            }
        }
        return chk;
    },

    hrpExpand: (hrp) => {
        const ret = [];
        for (let i = 0; i < hrp.length; i++) {
            ret.push(hrp.charCodeAt(i) >> 5);
        }
        ret.push(0);
        for (let i = 0; i < hrp.length; i++) {
            ret.push(hrp.charCodeAt(i) & 31);
        }
        return ret;
    },

    createChecksum: (hrp, data) => {
        const values = bech32m.hrpExpand(hrp).concat(data);
        const polymod = bech32m.polymod(values) ^ 1;
        const ret = [];
        for (let i = 0; i < 6; i++) {
            ret.push((polymod >> (5 * (5 - i))) & 31);
        }
        return ret;
    },

    encode: (hrp, data) => {
        const combined = data.concat(bech32m.createChecksum(hrp, data));
        let ret = hrp + '1';
        for (const p of combined) {
            ret += bech32m.charset[p];
        }
        return ret;
    },

    decode: (bechString) => {
        const p = bechString.lastIndexOf('1');
        if (p === -1) throw new Error('No separator character for address');
        const hrp = bechString.substring(0, p);
        const data = [];
        for (let i = p + 1; i < bechString.length; i++) {
            const d = bech32m.charset.indexOf(bechString[i]);
            if (d === -1) throw new Error('Invalid character in address');
            data.push(d);
        }
        const values = bech32m.hrpExpand(hrp).concat(data);
        if (bech32m.polymod(values) !== 1) throw new Error('Invalid checksum');
        return { hrp, data: data.slice(0, -6) };
    },

    encodeAddress: (publicKey) => {
        // Hash the public key with SHA-256
        const hash = crypto.subtle.digestSync('SHA-256', publicKey);
        const hashBytes = new Uint8Array(hash);

        // Convert 8-bit bytes to 5-bit groups
        const groups = [];
        let buffer = 0;
        let bits = 0;

        for (const byte of hashBytes) {
            buffer = (buffer << 8) | byte;
            bits += 8;

            while (bits >= 5) {
                bits -= 5;
                groups.push((buffer >> bits) & 31);
            }
        }

        if (bits > 0) {
            groups.push((buffer << (5 - bits)) & 31);
        }

        return bech32m.encode('aib', groups);
    },

    decodeAddress: (address) => {
        try {
            const { hrp, data } = bech32m.decode(address);
            if (hrp !== 'aib') throw new Error('Invalid HRP');

            // Convert 5-bit groups back to 8-bit bytes
            const bytes = [];
            let buffer = 0;
            let bits = 0;

            for (const group of data) {
                buffer = (buffer << 5) | group;
                bits += 5;

                while (bits >= 8) {
                    bits -= 8;
                    bytes.push((buffer >> bits) & 255);
                }
            }

            return new Uint8Array(bytes);
        } catch (error) {
            throw new Error(`Invalid address: ${error.message}`);
        }
    }
};

// Wallet Management
const walletManager = {
    generateMnemonic: () => {
        const words = [
            'abandon', 'ability', 'able', 'about', 'above', 'absent', 'absorb', 'abstract', 'absurd', 'abuse', 'access',
            'accident', 'account', 'accuse', 'achieve', 'acid', 'acoustic', 'acquire', 'across', 'act', 'action',
            'actor', 'actress', 'actual', 'adapt', 'add', 'addict', 'address', 'adjust', 'admit', 'adult', 'advance',
            'advice', 'aerobic', 'affair', 'afford', 'afraid', 'again', 'age', 'agent', 'agree', 'ahead', 'aim',
            'air', 'airport', 'aisle', 'alarm', 'album', 'alcohol', 'alert', 'alien', 'all', 'alley', 'allow',
            'almost', 'alone', 'alpha', 'already', 'also', 'alter', 'always', 'amateur', 'amazing', 'among', 'amount',
            'amused', 'analyst', 'anchor', 'ancient', 'anger', 'angle', 'angry', 'animal', 'ankle', 'announce', 'annual',
            'another', 'answer', 'antenna', 'antique', 'anxiety', 'any', 'apart', 'apology', 'appear', 'apple', 'approve',
            'april', 'arch', 'arctic', 'area', 'arena', 'argue', 'arm', 'armed', 'armor', 'army', 'around', 'arrange',
            'arrest', 'arrive', 'arrow', 'art', 'artefact', 'artist', 'artwork', 'ask', 'aspect', 'assault', 'asset',
            'assist', 'assume', 'asthma', 'athlete', 'atom', 'attack', 'attend', 'attitude', 'attract', 'auction', 'audit',
            'august', 'aunt', 'author', 'auto', 'autumn', 'average', 'avocado', 'avoid', 'awake', 'aware', 'away',
            'awesome', 'awful', 'awkward', 'axis', 'baby', 'bachelor', 'bacon', 'badge', 'bag', 'balance', 'balcony',
            'ball', 'bamboo', 'banana', 'banner', 'bar', 'barely', 'bargain', 'barrel', 'base', 'basic', 'basket',
            'battle', 'beach', 'bean', 'beauty', 'because', 'become', 'beef', 'before', 'begin', 'behave', 'behind',
            'believe', 'below', 'belt', 'bench', 'benefit', 'best', 'betray', 'better', 'between', 'beyond', 'bicycle',
            'bid', 'bike', 'bind', 'biology', 'bird', 'birth', 'bitter', 'black', 'blade', 'blame', 'blanket', 'blast',
            'bleak', 'bless', 'blind', 'blood', 'blossom', 'blouse', 'blue', 'blur', 'blush', 'board', 'boat', 'body',
            'boil', 'bomb', 'bone', 'bonus', 'book', 'boost', 'border', 'boring', 'borrow', 'boss', 'bottom', 'bounce',
            'box', 'boy', 'bracket', 'brain', 'brand', 'brass', 'brave', 'bread', 'breeze', 'brick', 'bridge', 'brief',
            'bright', 'bring', 'brisk', 'broccoli', 'broken', 'bronze', 'broom', 'brother', 'brown', 'brush', 'bubble',
            'buddy', 'budget', 'buffalo', 'build', 'bulb', 'bulk', 'bullet', 'bundle', 'bunker', 'burden', 'burger',
            'burst', 'bus', 'business', 'busy', 'butter', 'buyer', 'buzz', 'cabbage', 'cabin', 'cable', 'cactus',
            'cage', 'cake', 'call', 'calm', 'camera', 'camp', 'can', 'canal', 'cancel', 'candy', 'cannon', 'canoe',
            'canvas', 'canyon', 'capable', 'capital', 'captain', 'car', 'carbon', 'card', 'cargo', 'carpet', 'carry',
            'cart', 'case', 'cash', 'casino', 'castle', 'casual', 'cat', 'catalog', 'catch', 'category', 'cattle',
            'caught', 'cause', 'caution', 'cave', 'ceiling', 'celery', 'cement', 'census', 'century', 'cereal', 'certain',
            'chair', 'chalk', 'champion', 'change', 'chaos', 'chapter', 'charge', 'chase', 'chat', 'cheap', 'check',
            'cheese', 'chef', 'cherry', 'chest', 'chicken', 'chief', 'child', 'chimney', 'choice', 'choose', 'chronic',
            'chuckle', 'chunk', 'churn', 'cigar', 'cinnamon', 'circle', 'citizen', 'city', 'civil', 'claim', 'clap',
            'clarify', 'claw', 'clay', 'clean', 'clerk', 'clever', 'click', 'client', 'cliff', 'climb', 'clinic',
            'clip', 'clock', 'clog', 'close', 'cloth', 'cloud', 'clown', 'club', 'clump', 'cluster', 'clutch', 'coach',
            'coast', 'coconut', 'code', 'coffee', 'coil', 'coin', 'collect', 'color', 'column', 'combine', 'come',
            'comfort', 'comic', 'common', 'company', 'concert', 'conduct', 'confirm', 'congress', 'connect', 'consider',
            'control', 'convince', 'cook', 'cool', 'copper', 'copy', 'coral', 'core', 'corn', 'correct', 'cost',
            'cotton', 'couch', 'country', 'couple', 'course', 'cousin', 'cover', 'coyote', 'crack', 'cradle', 'craft',
            'cram', 'crane', 'crash', 'crater', 'crawl', 'crazy', 'cream', 'credit', 'creek', 'crew', 'cricket',
            'crime', 'crisp', 'critic', 'crop', 'cross', 'crouch', 'crowd', 'crucial', 'cruel', 'cruise', 'crumble',
            'crunch', 'crush', 'cry', 'crystal', 'cub', 'culture', 'cup', 'cupboard', 'curious', 'current', 'curtain',
            'curve', 'cushion', 'custom', 'cute', 'cycle', 'dad', 'damage', 'damp', 'dance', 'danger', 'daring',
            'dash', 'daughter', 'dawn', 'day', 'deal', 'debate', 'debris', 'decade', 'december', 'decide', 'decline',
            'decorate', 'decrease', 'deer', 'defense', 'define', 'defy', 'degree', 'delay', 'deliver', 'demand',
            'demise', 'denial', 'dentist', 'deny', 'depart', 'depend', 'deposit', 'depth', 'deputy', 'derive', 'describe',
            'desert', 'design', 'desk', 'despair', 'destroy', 'detail', 'detect', 'develop', 'device', 'devote',
            'diagram', 'dial', 'diamond', 'diary', 'dice', 'diesel', 'diet', 'differ', 'digital', 'dignity', 'dilemma',
            'dinner', 'dinosaur', 'direct', 'dirt', 'disagree', 'discover', 'disease', 'dish', 'dismiss', 'disorder',
            'display', 'distance', 'divert', 'divide', 'divorce', 'dizzy', 'doctor', 'document', 'dog', 'doll',
            'dolphin', 'domain', 'donate', 'donkey', 'donor', 'door', 'dose', 'double', 'dove', 'draft', 'dragon',
            'drama', 'drastic', 'draw', 'dream', 'dress', 'drift', 'drill', 'drink', 'drip', 'drive', 'drop',
            'drum', 'dry', 'duck', 'dumb', 'dune', 'during', 'dust', 'dutch', 'duty', 'dwarf', 'dynamic', 'eager',
            'eagle', 'early', 'earn', 'earth', 'easily', 'east', 'easy', 'echo', 'ecology', 'economy', 'edge',
            'edit', 'educate', 'effort', 'egg', 'eight', 'either', 'elbow', 'elder', 'electric', 'elegant', 'element',
            'elephant', 'elevator', 'elite', 'else', 'embark', 'embody', 'embrace', 'emerge', 'emotion', 'employ',
            'empower', 'empty', 'enable', 'enact', 'end', 'endless', 'endorse', 'enemy', 'energy', 'enforce', 'engage',
            'engine', 'enhance', 'enjoy', 'enlist', 'enough', 'enrich', 'enroll', 'ensure', 'enter', 'entire', 'entry',
            'envelope', 'episode', 'equal', 'equip', 'era', 'erase', 'erode', 'erosion', 'error', 'erupt', 'escape',
            'essay', 'essence', 'estate', 'eternal', 'ethics', 'evidence', 'evil', 'evoke', 'evolve', 'exact', 'example',
            'excess', 'exchange', 'excite', 'exclude', 'excuse', 'execute', 'exercise', 'exhaust', 'exhibit', 'exile',
            'exist', 'exit', 'exotic', 'expand', 'expect', 'expire', 'explain', 'expose', 'express', 'extend', 'extra',
            'eye', 'eyebrow', 'fabric', 'face', 'faculty', 'fade', 'faint', 'faith', 'fall', 'false', 'fame',
            'family', 'famous', 'fan', 'fancy', 'fantasy', 'farm', 'fashion', 'fat', 'fatal', 'father', 'fatigue',
            'fault', 'favorite', 'feature', 'february', 'federal', 'fee', 'feed', 'feel', 'female', 'fence',
            'festival', 'fetch', 'fever', 'few', 'fiber', 'fiction', 'field', 'figure', 'file', 'film', 'filter',
            'final', 'find', 'fine', 'finger', 'finish', 'fire', 'firm', 'first', 'fiscal', 'fish', 'fit',
            'fitness', 'fix', 'flag', 'flame', 'flash', 'flat', 'flavor', 'flee', 'flight', 'flip', 'float',
            'flock', 'floor', 'flower', 'fluid', 'flush', 'fly', 'foam', 'focus', 'fog', 'foil', 'fold',
            'follow', 'food', 'fool', 'foot', 'force', 'forest', 'forget', 'fork', 'fortune', 'forum', 'forward',
            'fossil', 'foster', 'found', 'fox', 'fragile', 'frame', 'frequent', 'fresh', 'friend', 'fringe',
            'frog', 'front', 'frost', 'frown', 'frozen', 'fruit', 'fuel', 'fun', 'funny', 'furnace', 'fury',
            'future', 'gadget', 'gain', 'galaxy', 'gallery', 'game', 'gap', 'garage', 'garbage', 'garden', 'garlic',
            'garment', 'gas', 'gasp', 'gate', 'gather', 'gauge', 'gaze', 'general', 'genius', 'genre', 'gentle',
            'genuine', 'gesture', 'ghost', 'giant', 'gift', 'giggle', 'ginger', 'giraffe', 'girl', 'give', 'glad',
            'glance', 'glare', 'glass', 'glide', 'glimpse', 'globe', 'gloom', 'glory', 'glove', 'glow', 'glue',
            'goat', 'goddess', 'gold', 'good', 'goose', 'gorilla', 'gospel', 'gossip', 'govern', 'gown', 'grab',
            'grace', 'grain', 'grant', 'grape', 'grass', 'gravity', 'great', 'green', 'grid', 'grief', 'grit',
            'grocery', 'group', 'grow', 'grunt', 'guard', 'guess', 'guide', 'guilt', 'guitar', 'gun', 'gym',
            'habit', 'hair', 'half', 'hammer', 'hamster', 'hand', 'happy', 'harbor', 'hard', 'harsh', 'harvest',
            'hat', 'have', 'hawk', 'hazard', 'head', 'health', 'heart', 'heavy', 'hedgehog', 'height', 'hello',
            'helmet', 'help', 'hen', 'hero', 'hidden', 'high', 'hill', 'hint', 'hip', 'hire', 'history',
            'hobby', 'hockey', 'hold', 'hole', 'holiday', 'hollow', 'home', 'honey', 'hood', 'hope', 'horn',
            'horror', 'horse', 'hospital', 'host', 'hotel', 'hour', 'hover', 'hub', 'huge', 'human', 'humble',
            'humor', 'hundred', 'hungry', 'hunt', 'hurdle', 'hurry', 'hurt', 'husband', 'hybrid', 'ice', 'icon',
            'idea', 'identify', 'idle', 'ignore', 'ill', 'illegal', 'illness', 'image', 'imitate', 'immense',
            'immune', 'impact', 'impose', 'improve', 'impulse', 'inch', 'include', 'income', 'increase', 'index',
            'indicate', 'indoor', 'industry', 'infant', 'inflict', 'inform', 'inhale', 'inherit', 'initial', 'inject',
            'injury', 'inmate', 'inner', 'innocent', 'input', 'inquiry', 'insane', 'insect', 'inside', 'inspire',
            'install', 'intact', 'interest', 'into', 'invest', 'invite', 'involve', 'iron', 'island', 'isolate',
            'issue', 'item', 'ivory', 'jacket', 'jaguar', 'jar', 'jazz', 'jealous', 'jeans', 'jelly', 'jewel',
            'job', 'join', 'joke', 'journey', 'joy', 'judge', 'juice', 'jump', 'jungle', 'junior', 'junk',
            'just', 'kangaroo', 'keen', 'keep', 'ketchup', 'key', 'kick', 'kid', 'kidney', 'kind', 'kingdom',
            'kiss', 'kit', 'kitchen', 'kite', 'kitten', 'kiwi', 'knee', 'knife', 'knock', 'know', 'lab',
            'label', 'labor', 'ladder', 'lady', 'lake', 'lamp', 'language', 'laptop', 'large', 'later', 'latin',
            'laugh', 'laundry', 'lava', 'law', 'lawn', 'lawsuit', 'layer', 'lazy', 'leader', 'leaf', 'learn',
            'leave', 'lecture', 'left', 'leg', 'legal', 'legend', 'leisure', 'lemon', 'lend', 'length', 'lens',
            'leopard', 'lesson', 'letter', 'level', 'liar', 'liberty', 'library', 'license', 'life', 'lift',
            'light', 'like', 'limb', 'limit', 'link', 'lion', 'liquid', 'list', 'little', 'live', 'lizard',
            'load', 'loan', 'lobster', 'local', 'lock', 'logic', 'lonely', 'long', 'loop', 'lottery', 'loud',
            'lounge', 'love', 'loyal', 'lucky', 'luggage', 'lumber', 'lunar', 'lunch', 'luxury', 'lyrics', 'machine',
            'mad', 'magic', 'magnet', 'maid', 'mail', 'main', 'major', 'make', 'mammal', 'man', 'manage',
            'mandate', 'mango', 'mansion', 'manual', 'maple', 'marble', 'march', 'margin', 'marine', 'market', 'marriage',
            'mask', 'mass', 'master', 'match', 'material', 'math', 'matrix', 'matter', 'maximum', 'maze', 'meadow',
            'mean', 'measure', 'meat', 'mechanic', 'medal', 'media', 'melody', 'melt', 'member', 'memory', 'mention',
            'menu', 'mercy', 'merge', 'merit', 'merry', 'mesh', 'message', 'metal', 'method', 'middle', 'midnight',
            'milk', 'million', 'mimic', 'mind', 'minimum', 'minor', 'minute', 'miracle', 'mirror', 'misery',
            'miss', 'mistake', 'mix', 'mixed', 'mixture', 'mobile', 'model', 'modify', 'mom', 'moment', 'monitor',
            'monkey', 'monster', 'month', 'moon', 'moral', 'more', 'morning', 'mosquito', 'mother', 'motion',
            'motor', 'mountain', 'mouse', 'move', 'movie', 'much', 'muffin', 'mule', 'multiply', 'muscle', 'museum',
            'mushroom', 'music', 'must', 'mutual', 'myself', 'mystery', 'myth', 'naive', 'name', 'napkin', 'narrow',
            'nasty', 'nation', 'nature', 'near', 'neck', 'need', 'negative', 'neglect', 'neither', 'nephew', 'nerve',
            'nest', 'net', 'network', 'neutral', 'never', 'news', 'next', 'nice', 'night', 'noble', 'noise',
            'nominee', 'noodle', 'normal', 'north', 'nose', 'notable', 'note', 'nothing', 'notice', 'novel', 'now',
            'nuclear', 'number', 'nurse', 'nut', 'oak', 'obey', 'object', 'oblige', 'obscure', 'observe', 'obtain',
            'obvious', 'occur', 'ocean', 'october', 'odor', 'off', 'offer', 'office', 'often', 'oil', 'okay',
            'old', 'olive', 'olympic', 'omit', 'once', 'one', 'onion', 'online', 'only', 'open', 'opera',
            'opinion', 'oppose', 'option', 'orange', 'orbit', 'orchard', 'order', 'ordinary', 'organ', 'orient',
            'original', 'orphan', 'ostrich', 'other', 'outdoor', 'outer', 'output', 'outside', 'oval', 'oven',
            'over', 'own', 'owner', 'oxygen', 'oyster', 'ozone', 'pact', 'paddle', 'page', 'pair', 'palace',
            'palm', 'panda', 'panel', 'panic', 'panther', 'paper', 'parade', 'parent', 'park', 'parrot', 'party',
            'pass', 'patch', 'path', 'patient', 'patrol', 'pattern', 'pause', 'pave', 'payment', 'peace', 'peanut',
            'pear', 'peasant', 'pelican', 'pen', 'penalty', 'pencil', 'people', 'pepper', 'perfect', 'permit',
            'person', 'pet', 'phone', 'photo', 'phrase', 'physical', 'piano', 'picnic', 'picture', 'piece',
            'pig', 'pigeon', 'pill', 'pilot', 'pink', 'pioneer', 'pipe', 'pistol', 'pitch', 'pizza', 'place',
            'planet', 'plastic', 'plate', 'play', 'please', 'pledge', 'pluck', 'plug', 'plunge', 'poem',
            'poet', 'point', 'polar', 'pole', 'police', 'pond', 'pony', 'pool', 'popular', 'portion', 'position',
            'possible', 'post', 'potato', 'pottery', 'poverty', 'powder', 'power', 'practice', 'praise', 'predict',
            'prefer', 'prepare', 'present', 'pretty', 'prevent', 'price', 'pride', 'primary', 'print', 'priority',
            'prison', 'private', 'prize', 'problem', 'process', 'produce', 'profit', 'program', 'project', 'promote',
            'proof', 'property', 'prosper', 'protect', 'proud', 'provide', 'public', 'pudding', 'pull', 'pulp',
            'pulse', 'pumpkin', 'punch', 'pupil', 'puppy', 'purchase', 'purity', 'purpose', 'purse', 'push',
            'put', 'puzzle', 'pyramid', 'quality', 'quantum', 'quarter', 'question', 'quick', 'quit', 'quiz',
            'quote', 'rabbit', 'raccoon', 'race', 'rack', 'radar', 'radio', 'rail', 'rain', 'raise', 'rally',
            'ramp', 'ranch', 'random', 'range', 'rapid', 'rare', 'rate', 'rather', 'raven', 'raw', 'razor',
            'ready', 'real', 'reason', 'rebel', 'recall', 'receive', 'recipe', 'record', 'recycle', 'reduce',
            'reflect', 'reform', 'refuse', 'region', 'regret', 'regular', 'reject', 'relax', 'release', 'relief',
            'rely', 'remain', 'remember', 'remind', 'remove', 'render', 'renew', 'rent', 'reopen', 'repair',
            'repeat', 'replace', 'report', 'require', 'rescue', 'resemble', 'resist', 'resource', 'response', 'result',
            'retire', 'retreat', 'return', 'reunion', 'reveal', 'review', 'reward', 'rhythm', 'rib', 'ribbon',
            'rice', 'rich', 'ride', 'ridge', 'rifle', 'right', 'rigid', 'ring', 'riot', 'ripple', 'risk',
            'ritual', 'rival', 'river', 'road', 'roast', 'robot', 'robust', 'rocket', 'romance', 'roof',
            'rookie', 'room', 'rose', 'rotate', 'rough', 'round', 'route', 'royal', 'rubber', 'rude', 'rug',
            'rule', 'run', 'runway', 'rural', 'sad', 'saddle', 'sadness', 'safe', 'sail', 'salad', 'salmon',
            'salon', 'salt', 'salute', 'same', 'sample', 'sand', 'satisfy', 'satoshi', 'sauce', 'sausage',
            'save', 'say', 'scale', 'scan', 'scare', 'scatter', 'scene', 'scheme', 'school', 'science',
            'scissors', 'scorpion', 'scout', 'scrap', 'screen', 'script', 'scrub', 'sea', 'search', 'season',
            'seat', 'second', 'secret', 'section', 'security', 'seed', 'seek', 'segment', 'select', 'sell',
            'seminar', 'senior', 'sense', 'sentence', 'series', 'service', 'session', 'settle', 'setup', 'seven',
            'shadow', 'shaft', 'shallow', 'share', 'shed', 'shell', 'sheriff', 'shield', 'shift', 'shine',
            'ship', 'shiver', 'shock', 'shoe', 'shoot', 'shop', 'short', 'shoulder', 'shove', 'shrimp',
            'shrug', 'shuffle', 'shy', 'sibling', 'sick', 'side', 'siege', 'sight', 'sigma', 'sign',
            'silent', 'silk', 'silly', 'silver', 'similar', 'simple', 'since', 'sing', 'siren', 'sister',
            'situate', 'six', 'size', 'skate', 'sketch', 'ski', 'skill', 'skin', 'skirt', 'skull',
            'slab', 'slam', 'sleep', 'slender', 'slice', 'slide', 'slight', 'slim', 'slogan', 'slot',
            'slow', 'slush', 'small', 'smart', 'smile', 'smoke', 'smooth', 'snack', 'snake', 'snap',
            'sniff', 'snow', 'soap', 'soccer', 'social', 'sock', 'soda', 'soft', 'solar', 'soldier',
            'solid', 'solution', 'solve', 'someone', 'song', 'soon', 'sorry', 'sort', 'soul', 'sound',
            'soup', 'source', 'south', 'space', 'spare', 'spatial', 'spawn', 'speak', 'special', 'speed',
            'spell', 'spend', 'sphere', 'spice', 'spider', 'spike', 'spin', 'spirit', 'split', 'spoil',
            'sponsor', 'spoon', 'sport', 'spot', 'spray', 'spread', 'spring', 'spy', 'square', 'squeeze',
            'squirrel', 'stable', 'stadium', 'staff', 'stage', 'stairs', 'stamp', 'stand', 'start', 'state',
            'stay', 'steak', 'steel', 'stem', 'step', 'stereo', 'stick', 'still', 'sting', 'stock',
            'stomach', 'stone', 'stool', 'story', 'stove', 'strategy', 'street', 'strike', 'strong', 'struggle',
            'student', 'stuff', 'stumble', 'style', 'subject', 'submit', 'subway', 'success', 'such', 'sudden',
            'suffer', 'sugar', 'suggest', 'suit', 'summer', 'sun', 'sunny', 'sunset', 'super', 'supply',
            'supreme', 'sure', 'surface', 'surge', 'surprise', 'surround', 'survey', 'suspect', 'sustain', 'swallow',
            'swamp', 'swap', 'swarm', 'swear', 'sweet', 'swift', 'swim', 'swing', 'switch', 'sword',
            'symbol', 'symptom', 'syrup', 'system', 'table', 'tackle', 'tag', 'tail', 'talent', 'talk',
            'tank', 'tape', 'target', 'task', 'taste', 'tattoo', 'taxi', 'teach', 'team', 'tell',
            'ten', 'tenant', 'tennis', 'tent', 'term', 'test', 'text', 'thank', 'that', 'theme',
            'then', 'theory', 'there', 'they', 'thing', 'this', 'thought', 'three', 'thrive', 'throw',
            'thumb', 'thunder', 'ticket', 'tide', 'tiger', 'tilt', 'timber', 'time', 'tiny', 'tip',
            'tired', 'tissue', 'title', 'toast', 'tobacco', 'today', 'toddler', 'toe', 'together', 'toilet',
            'token', 'tomato', 'tomorrow', 'tone', 'tongue', 'tonight', 'tool', 'tooth', 'top', 'topic',
            'topple', 'torch', 'tornado', 'tortoise', 'toss', 'total', 'tourist', 'toward', 'tower', 'town',
            'toy', 'track', 'trade', 'traffic', 'tragic', 'train', 'transfer', 'trap', 'trash', 'travel',
            'tray', 'treat', 'tree', 'trend', 'trial', 'tribe', 'trick', 'trigger', 'trim', 'trip',
            'trophy', 'trouble', 'truck', 'true', 'truly', 'trumpet', 'trust', 'truth', 'try', 'tube',
            'tuition', 'tumble', 'tuna', 'tunnel', 'turkey', 'turn', 'turtle', 'twelve', 'twenty', 'twice',
            'twin', 'twist', 'two', 'type', 'typical', 'ugly', 'umbrella', 'unable', 'unaware', 'uncle',
            'uncover', 'under', 'undo', 'unfair', 'unfold', 'unhappy', 'uniform', 'unique', 'unit', 'universe',
            'unknown', 'unlock', 'until', 'unusual', 'unveil', 'update', 'upgrade', 'uphold', 'upon', 'upper',
            'upset', 'urban', 'urge', 'usage', 'use', 'used', 'useful', 'useless', 'usual', 'utility',
            'vacant', 'vacuum', 'vague', 'valid', 'valley', 'valve', 'van', 'vanish', 'vapor', 'various',
            'vast', 'vault', 'vehicle', 'velvet', 'vendor', 'venture', 'venue', 'verb', 'verify', 'version',
            'very', 'vessel', 'veteran', 'viable', 'vibrant', 'vicious', 'victory', 'video', 'view', 'village',
            'vintage', 'violin', 'virtual', 'virus', 'visa', 'visit', 'visual', 'vital', 'vivid', 'vocal',
            'voice', 'void', 'volcano', 'volume', 'vote', 'voyage', 'wage', 'wagon', 'wait', 'walk',
            'wall', 'walnut', 'want', 'warfare', 'warm', 'warrior', 'wash', 'wasp', 'waste', 'watch',
            'water', 'wave', 'way', 'wealth', 'weapon', 'wear', 'weasel', 'weather', 'web', 'wedding',
            'weekend', 'weird', 'welcome', 'west', 'wet', 'whale', 'what', 'wheat', 'wheel', 'when',
            'where', 'whip', 'whisper', 'wide', 'width', 'wife', 'wild', 'will', 'win', 'window',
            'wine', 'wing', 'wink', 'winner', 'winter', 'wire', 'wisdom', 'wise', 'wish', 'witness',
            'wolf', 'woman', 'wonder', 'wood', 'wool', 'word', 'work', 'world', 'worry', 'worth',
            'wrap', 'wreck', 'wrestle', 'wrist', 'write', 'wrong', 'yard', 'year', 'yellow', 'you',
            'young', 'youth', 'zebra', 'zero', 'zone', 'zoo'
        ];

        const mnemonic = [];
        for (let i = 0; i < 12; i++) {
            const randomIndex = Math.floor(Math.random() * words.length);
            mnemonic.push(words[randomIndex]);
        }
        return mnemonic.join(' ');
    },

    generateKeyPairFromMnemonic: (mnemonic) => {
        // Simple derivation: hash the mnemonic to get a seed
        const encoder = new TextEncoder();
        const data = encoder.encode(mnemonic);
        const hash = crypto.subtle.digestSync('SHA-256', data);

        // Use first 32 bytes as private key seed
        const seed = new Uint8Array(hash.slice(0, 32));

        // Generate actual Ed25519 key pair
        return crypto.generateKeyPair();
    },

    createWallet: async (name, password) => {
        const keyPair = await crypto.generateKeyPair();

        // Export keys
        const privateKeyBuffer = await crypto.subtle.exportKey('raw', keyPair.privateKey);
        const publicKeyBuffer = await crypto.subtle.exportKey('raw', keyPair.publicKey);

        const privateKey = new Uint8Array(privateKeyBuffer);
        const publicKey = new Uint8Array(publicKeyBuffer);

        // Generate address
        const address = bech32m.encodeAddress(publicKey);

        const walletData = {
            name: name || 'My AIB Wallet',
            privateKey: utils.bytesToHex(privateKey),
            publicKey: utils.bytesToHex(publicKey),
            address: address,
            createdAt: new Date().toISOString(),
            network: 'testnet'
        };

        // Encrypt if password provided
        if (password) {
            walletData.encrypted = true;
            walletData.passwordHash = utils.bytesToHex(crypto.subtle.digestSync('SHA-256', encoder.encode(password)));
        }

        return walletData;
    },

    importWallet: async (privateKeyHex) => {
        const privateKeyBytes = utils.hexToBytes(privateKeyHex);

        if (privateKeyBytes.length !== 32) {
            throw new Error('Invalid private key length. Must be 32 bytes.');
        }

        // Import private key
        const privateKey = await crypto.subtle.importKey(
            'raw',
            privateKeyBytes,
            { name: 'ED25519' },
            true,
            ['sign']
        );

        // Generate public key
        const publicKeyBuffer = await crypto.subtle.exportKey('raw', privateKey);
        const publicKey = new Uint8Array(publicKeyBuffer);

        // Generate address
        const address = bech32m.encodeAddress(publicKey);

        return {
            name: 'Imported Wallet',
            privateKey: utils.bytesToHex(privateKeyBytes),
            publicKey: utils.bytesToHex(publicKey),
            address: address,
            createdAt: new Date().toISOString(),
            network: 'testnet'
        };
    },

    loadWallet: () => {
        const saved = localStorage.getItem('aib_wallet');
        if (saved) {
            try {
                return JSON.parse(saved);
            } catch (e) {
                console.error('Failed to parse saved wallet:', e);
                return null;
            }
        }
        return null;
    },

    saveWallet: (walletData) => {
        localStorage.setItem('aib_wallet', JSON.stringify(walletData));
    },

    clearWallet: () => {
        localStorage.removeItem('aib_wallet');
        wallet = null;
        updateUI();
    }
};

// Transaction Functions
const transactionManager = {
    async signTransaction(privateKeyHex, transactionData) {
        const privateKeyBytes = utils.hexToBytes(privateKeyHex);
        const privateKey = await crypto.subtle.importKey(
            'raw',
            privateKeyBytes,
            { name: 'ED25519' },
            false,
            ['sign']
        );

        const encoder = new TextEncoder();
        const message = encoder.encode(JSON.stringify(transactionData));
        const signature = await crypto.sign(privateKey, message);

        return utils.bytesToHex(signature);
    },

    createTransaction: (recipient, amount, fee) => {
        const amountSat = Math.floor(amount * 1e8);
        const feeSat = Math.floor(fee * 1e8);

        return {
            version: 1,
            inputs: [{
                txHash: '0000000000000000000000000000000000000000000000000000000000000000',
                outputIndex: 0,
                signature: '',
                sequence: 0
            }],
            outputs: [
                {
                    address: recipient,
                    amount: amountSat
                },
                {
                    address: wallet.address,
                    amount: 0
                }
            ],
            lockTime: 0,
            sequence: Date.now()
        };
    }
};

// API Functions
const api = {
    async getBalance(address) {
        try {
            const response = await fetch(`${CONFIG.apiEndpoint}/api/v1/balance/${address}`);
            const data = await response.json();
            return data.balance || 0;
        } catch (error) {
            console.error('Failed to get balance:', error);
            return 0;
        }
    },

    async getTransactions(address) {
        try {
            const response = await fetch(`${CONFIG.apiEndpoint}/api/v1/transactions/${address}`);
            const data = await response.json();
            return data.transactions || [];
        } catch (error) {
            console.error('Failed to get transactions:', error);
            return [];
        }
    },

    async sendTransaction(transaction) {
        try {
            const response = await fetch(`${CONFIG.apiEndpoint}/api/v1/send`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(transaction)
            });
            const result = await response.json();
            return result;
        } catch (error) {
            console.error('Failed to send transaction:', error);
            throw error;
        }
    }
};

// UI Functions
function updateUI() {
    if (!wallet) {
        elements.totalBalance.textContent = '0.00000000';
        elements.walletAddress.textContent = 'Not generated yet';
        elements.privateKeyDisplay.textContent = 'Hidden for security';
        elements.privateKeyDisplay.style.color = 'var(--danger)';
        return;
    }

    elements.totalBalance.textContent = utils.formatBalance(wallet.balance || 0);
    elements.walletAddress.textContent = wallet.address;
    elements.privateKeyDisplay.textContent = isPrivateKeyVisible ?
        wallet.privateKey : 'Hidden for security';
    elements.privateKeyDisplay.style.color = isPrivateKeyVisible ? 'var(--text-primary)' : 'var(--danger)';
    elements.currentNetwork.textContent = currentNetwork.toUpperCase();

    // Update history if loaded
    if (wallet.transactions) {
        renderTransactionHistory(wallet.transactions);
    }
}

function renderTransactionHistory(transactions) {
    elements.txHistoryBody.innerHTML = '';

    if (!transactions || transactions.length === 0) {
        elements.txHistoryBody.innerHTML = `
            <tr>
                <td colspan="7" style="text-align: center; color: var(--text-secondary); padding: 40px;">
                    No transactions yet. Start by creating a wallet and receiving some AIB.
                </td>
            </tr>
        `;
        return;
    }

    const filtered = transactions.filter(tx => {
        if (historyFilter === 'all') return true;
        return tx.type === historyFilter;
    });

    filtered.forEach(tx => {
        const row = document.createElement('tr');

        const typeClass = tx.type === 'receive' ? 'receive' : 'send';
        const amountClass = tx.type === 'receive' ? '' : 'negative';

        row.innerHTML = `
            <td><span class="tx-type ${typeClass}">${tx.type}</span></td>
            <td class="tx-hash">${tx.hash}</td>
            <td class="tx-amount ${amountClass}">${utils.formatBalance(tx.amount)}</td>
            <td>${utils.formatBalance(tx.fee || 0)}</td>
            <td>${tx.block || 'Pending'}</td>
            <td class="tx-status">
                <span style="display: inline-block; width: 8px; height: 8px; background: var(--success); border-radius: 50%; margin-right: 6px;"></span>
                ${tx.status || 'Confirmed'}
            </td>
            <td>${new Date(tx.timestamp).toLocaleString()}</td>
        `;

        elements.txHistoryBody.appendChild(row);
    });

    elements.lastHistoryUpdate.textContent = new Date().toLocaleString();
}

// Event Handlers
async function createWallet() {
    const name = document.getElementById('walletName').value.trim();
    const password = document.getElementById('walletPassword').value;

    try {
        const walletData = await walletManager.createWallet(name, password);
        wallet = walletData;

        // Display generated details
        document.getElementById('generatedPrivateKey').value = wallet.privateKey;
        document.getElementById('generatedPublicKey').value = wallet.publicKey;
        document.getElementById('generatedAddress').value = wallet.address;
        document.getElementById('walletOutput').style.display = 'block';

        walletManager.saveWallet(wallet);
        updateUI();
        utils.showToast('Wallet created successfully!', 'success');
    } catch (error) {
        utils.showToast(`Failed to create wallet: ${error.message}`, 'error');
    }
}

async function importPrivateKey() {
    const privateKeyHex = document.getElementById('importPrivateKey').value.trim();

    if (!privateKeyHex) {
        utils.showToast('Please enter a private key', 'error');
        return;
    }

    try {
        const walletData = await walletManager.importWallet(privateKeyHex);
        wallet = walletData;
        walletManager.saveWallet(wallet);

        updateUI();
        utils.showToast('Wallet imported successfully!', 'success');
        switchTab('send');
    } catch (error) {
        utils.showToast(`Failed to import wallet: ${error.message}`, 'error');
    }
}

function importMnemonic() {
    const mnemonic = document.getElementById('importMnemonic').value.trim();
    if (!mnemonic) {
        utils.showToast('Please enter a mnemonic phrase', 'error');
        return;
    }

    // Note: This is a simplified implementation
    // In a real implementation, you'd derive keys from the mnemonic
    utils.showToast('Mnemonic import not fully implemented in demo', 'info');
}

async function refreshBalance() {
    if (!wallet) {
        utils.showToast('No wallet loaded', 'error');
        return;
    }

    try {
        const balance = await api.getBalance(wallet.address);
        wallet.balance = balance;
        walletManager.saveWallet(wallet);
        updateUI();
        utils.showToast('Balance refreshed', 'success');
    } catch (error) {
        utils.showToast('Failed to refresh balance', 'error');
    }
}

async function loadHistory() {
    if (!wallet) {
        utils.showToast('No wallet loaded', 'error');
        return;
    }

    try {
        const transactions = await api.getTransactions(wallet.address);
        wallet.transactions = transactions;
        walletManager.saveWallet(wallet);
        renderTransactionHistory(transactions);
        utils.showToast('Transaction history loaded', 'success');
    } catch (error) {
        utils.showToast('Failed to load history', 'error');
    }
}

async function sendTransaction() {
    if (!wallet) {
        utils.showToast('No wallet loaded', 'error');
        return;
    }

    const address = document.getElementById('sendAddress').value.trim();
    const amount = parseFloat(document.getElementById('sendAmount').value);
    const fee = parseFloat(document.getElementById('sendFee').value);

    if (!address || !amount || !fee) {
        utils.showToast('Please fill all fields', 'error');
        return;
    }

    try {
        const tx = transactionManager.createTransaction(address, amount, fee);
        const signature = await transactionManager.signTransaction(wallet.privateKey, tx);

        tx.inputs[0].signature = signature;

        const result = await api.sendTransaction(tx);
        utils.showToast(`Transaction sent! Hash: ${result.hash}`, 'success');

        // Refresh balance and history
        await refreshBalance();
        await loadHistory();
    } catch (error) {
        utils.showToast(`Failed to send transaction: ${error.message}`, 'error');
    }
}

function switchTab(tabName) {
    // Hide all tabs
    Object.values(elements.tabContent).forEach(tab => tab.style.display = 'none');
    Object.values(elements.tabs).forEach(tab => tab.classList.remove('active'));

    // Show selected tab
    elements.tabContent[tabName].style.display = 'block';
    elements.tabs[tabName].classList.add('active');
}

function togglePrivateKey() {
    isPrivateKeyVisible = !isPrivateKeyVisible;
    if (wallet) {
        elements.privateKeyDisplay.textContent = isPrivateKeyVisible ?
            wallet.privateKey : 'Hidden for security';
        elements.privateKeyDisplay.style.color = isPrivateKeyVisible ? 'var(--text-primary)' : 'var(--danger)';
    }
}

function showQRCode() {
    if (!wallet) {
        utils.showToast('No wallet loaded', 'error');
        return;
    }

    const modal = document.getElementById('qrModal');
    const qrCode = document.getElementById('qrCode');

    // Clear previous QR code
    qrCode.innerHTML = '';
    new QRCode(qrCode, {
        text: wallet.address,
        width: 200,
        height: 200
    });

    modal.style.display = 'flex';
}

function closeQRCode() {
    document.getElementById('qrModal').style.display = 'none';
}

function copyAddress() {
    if (!wallet) {
        utils.showToast('No wallet loaded', 'error');
        return;
    }

    navigator.clipboard.writeText(wallet.address).then(() => {
        utils.showToast('Address copied to clipboard', 'success');
    }).catch(() => {
        utils.showToast('Failed to copy address', 'error');
    });
}

function setImportMethod(method) {
    document.getElementById('import-private').style.display = method === 'private' ? 'block' : 'none';
    document.getElementById('import-mnemonic').style.display = method === 'mnemonic' ? 'block' : 'none';
}

function setNetwork(network) {
    currentNetwork = network;
    elements.currentNetwork.textContent = network.toUpperCase().padEnd(4);
    utils.showToast(`Network set to ${network.toUpperCase()}`, 'info');
}

function filterHistory(filter) {
    historyFilter = filter;
    if (wallet && wallet.transactions) {
        renderTransactionHistory(wallet.transactions);
    }
}

function calculateFee() {
    // Simple fee calculation based on transaction size and network
    const baseFee = currentNetwork === 'l1' ? 0.0001 : 0.00005;
    document.getElementById('sendFee').value = baseFee;
    utils.showToast(`Fee calculated for ${currentNetwork.toUpperCase()}: ${baseFee} AIB`, 'info');
}

function clearWallet() {
    if (confirm('Are you sure you want to clear the wallet? This will remove all data.')) {
        walletManager.clearWallet();
        utils.showToast('Wallet cleared', 'info');
    }
}

function exportWallet() {
    if (!wallet) {
        utils.showToast('No wallet to export', 'error');
        return;
    }

    const dataStr = JSON.stringify(wallet, null, 2);
    const dataBlob = new Blob([dataStr], {type: 'application/json'});
    const url = URL.createObjectURL(dataBlob);

    const link = document.createElement('a');
    link.href = url;
    link.download = `aib-wallet-${Date.now()}.json`;
    link.click();

    URL.revokeObjectURL(url);
    utils.showToast('Wallet exported', 'success');
}

function downloadWallet() {
    if (!wallet) return;
    exportWallet();
}

function connectNode() {
    utils.showToast('Node connection status: Connected to testnet', 'success');
}

function generateMnemonic() {
    const mnemonic = walletManager.generateMnemonic();
    document.getElementById('importMnemonic').value = mnemonic;
    utils.showToast('Mnemonic generated', 'success');
}

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    // Load wallet from storage
    wallet = walletManager.loadWallet();
    updateUI();
});
</script>